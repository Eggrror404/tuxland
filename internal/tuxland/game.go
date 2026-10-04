package tuxland

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// The supervision loop's timing. The normal path is not a timer at all — it is
// bash's own prompt (evIdle in session.go), so a student's Enter is answered as
// fast as the shell can turn around, and never into the middle of its output.
//
// The two waits below are the fallback for the one case that signal cannot cover:
// a bash that will never prompt again. They are guesses, and a guess here is
// expensive, because a merely slow command is not finished just because it has
// been quiet a moment. So both sit far longer than anything the game asks a
// player to run takes, and a fallback that fires anyway costs a late prompt, never a wrong
// screen. The measurement, and the two guesses it replaced: DESIGN.md §3.
const (
	quietGap   = 2 * time.Second // no prompt, and this long silent: call it done
	hardCap    = 4 * time.Second // ...or this long since the Enter, whatever it prints
	backstop   = time.Second     // slow commands: check anyway
	tickRate   = 25 * time.Millisecond
	hintAfter  = 2                       // the first failed try gets silence
	stuckAfter = 6                       // this many attempts in: name the way out
	nudgeEvery = 50 * time.Second        // a frozen student gets a nudge
	eofGrace   = 1200 * time.Millisecond // scripted run: let a step settle, then leave
	breakPause = 1200 * time.Millisecond // scripted run: nothing to press Enter for
)

var nudgePhrases = []string{"no rush", "still here", "small nudge"}

// game supervises one playthrough, one level at a time.
type game struct {
	ui   *ui
	kb   *keyboard
	dir  string
	sess *session
	prog *progress // what the student has finished, so the next run can offer it
	eof  time.Time // set when stdin ran out (piped/scripted run)
}

// play is a full run: the menu, the levels in order, then the exit ramp. start
// is a menu choice the caller already knows — zero means nobody has chosen yet,
// which is the normal way in.
//
// One keyboard lives for the whole run, the menu included: the menu reads its
// keys through it (askLevel), so there is never a second goroutine on stdin,
// however many times the menu comes back.
func play(u *ui, start int) {
	prog := loadProgress()
	kb := newKeyboard()
	defer kb.close()

	for {
		if start == 0 {
			u.kickoff(prog)
			var ok bool
			if start, ok = askLevel(u, kb, prog); !ok {
				return
			}
		}
		// The extra is the habits toolbox: chosen from the menu, no number, never
		// part of the sequence. Finish it and the menu comes back with its ✅ on.
		// A numbered run carries on through the levels after the chosen one, then
		// rolls into the finale.
		run, extra := numberedLevels(), false
		if start == len(levels) {
			run, extra, start = []*level{extraLevel()}, true, 0
		}
		for _, lv := range run {
			if lv.num < start {
				continue
			}
			g := &game{ui: u, kb: kb, prog: &prog}
			if !g.runLevel(lv) {
				return // the student left; the goodbye is already on screen
			}
		}
		if !extra {
			u.finale()
			return
		}
	}
}

// runLevel scaffolds a level's playground, opens a real bash in it, walks the
// steps, then the "press Enter for the next level" break. It reports whether
// the run should carry on.
func (g *game) runLevel(lv *level) bool {
	// A level whose cards type at a program this machine does not have is turned
	// away here, before anything is scaffolded: no card of it can be passed, so
	// the only alternative is a run that stalls on a hint ladder with no way
	// down. Saying so once, up front, is the honest version of that.
	if missing := missingTools(lv.needs); len(missing) > 0 {
		g.noTools(lv, missing)
		return false
	}
	dir, err := scaffoldLevel(lv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return false
	}
	sess, err := newSession(dir, g.ui)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return false
	}
	g.dir, g.sess = dir, sess
	// Hand the keyboard over now that bash is listening. Keystrokes that arrived
	// between two levels are dropped with it, so a stray Enter can't skip step one.
	g.kb.attach(sess)
	defer func() {
		g.kb.detach()
		sess.close()
	}()

	g.banner(lv)
	for i, sec := range lv.sections {
		g.sectionHeader(sec, i+1)
		for i := range sec.steps {
			if !g.runStep(&sec.steps[i]) {
				g.ui.bye(g.dir, false)
				return false
			}
		}
	}
	g.levelComplete(lv)
	if lv.isLast() {
		return true
	}
	if !g.waitBreak() {
		g.ui.bye(g.dir, true)
		return false
	}
	return true
}

// ── the game's voice ─────────────────────────────────────────────────────────

func (g *game) banner(lv *level) {
	u := g.ui
	bar := u.col(ansiBold+ansiCyan, u.rule("═", 44))
	first := "  " + lv.emoji + " "
	head := fmt.Sprintf("LEVEL %d · %s", lv.num, lv.title)
	if lv.isExtra() {
		head = "EXTRA · " + lv.title // the toolbox has no number to announce
	}
	u.lines("")
	u.line("  " + bar)
	u.proseWith(first, hangFor(first), span{head, ansiBold + ansiYellow})
	u.prose("  ", "  ", lv.lead)
	u.proseWith("  "+u.col(ansiDim, "your playground: "), "  ", span{tilde(g.dir), ansiBold + ansiCyan})
	u.dimline("nothing outside it can break.")
	u.line("  " + bar)
}

func (g *game) sectionHeader(sec section, n int) {
	u := g.ui
	head := fmt.Sprintf("── §%d · %s", n, sec.title)
	// Fill the rest of the line with a rule, as far as the terminal goes.
	if fill := min(48, u.width()-2) - widthOf(head) - 1; fill > 0 {
		head += " " + strings.Repeat("─", fill)
	}
	u.line("  " + u.col(ansiCyan, head))
	u.prose("  ", "  ", sec.lead)
}

// card prints the active step directly above the prompt, then hands the
// terminal back. The prompt lives here (bash's PS1 is a stripped marker) so a
// card and the shell's own output can never fight over the same line.
func (g *game) card(sym, goal, note string) {
	u := g.ui
	first := "  " + u.col(ansiGreen, sym) + " "
	u.lines("")
	u.prose(first, hangFor(first), goal)
	for _, l := range strings.Split(note, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		u.prose("      ", "      ", l)
	}
	u.prompt()
}

func (g *game) ok(msg string) {
	first := "  " + g.ui.col(ansiGreen, "✅") + " "
	g.ui.prose(first, hangFor(first), msg)
}

// warn says that something is not right yet and what to try instead. It does not
// hand the terminal back: the caller does, once, after everything it had to say
// in this beat — a hint and the way out are two lines and one prompt.
func (g *game) warn(prefix, hint string) {
	u := g.ui
	first := "  " + u.col(ansiYellow, "⚠") + " "
	u.proseWith(first, hangFor(first), span{prefix, ansiDim}, span{" — " + hint, ""})
}

// noTools is the game turning a level away: a program it needs is not installed.
// It names the program, says plainly that nothing was changed, and gives the one
// command to run — a workshop machine with no calculator should be told where to
// get one, not left staring at a card it cannot pass.
func (g *game) noTools(lv *level, missing []string) {
	u := g.ui
	head := "  " + u.col(ansiBold+ansiYellow, "⛔") + " "
	u.lines("")
	u.proseWith(head, hangFor(head), span{fmt.Sprintf("level %d cannot run here: this machine has no %s",
		lv.num, strings.Join(missing, " and no ")), ansiBold + ansiYellow})
	u.dimline("a level whose commands are missing has no step you could pass, so the game stops " +
		"here instead of stalling on the card. nothing has been changed.")
	u.prose("  ", "  ", "install "+strings.Join(missing, " and ")+", then start the level again:")
	for _, prog := range missing {
		for _, row := range [][2]string{
			{"Debian, Ubuntu", "sudo apt install " + prog},
			{"Fedora, RHEL", "sudo dnf install " + prog},
			{"Arch", "sudo pacman -S " + prog},
			{"macOS", "brew install " + prog},
		} {
			u.proseWith("      ", "      ", span{pad(row[0], 15), ansiDim}, span{"`" + row[1] + "`", ""})
		}
	}
	u.lines("")
}

// missingTools are the programs a level needs that this machine does not have.
func missingTools(progs []string) []string {
	var missing []string
	for _, prog := range progs {
		if _, err := exec.LookPath(prog); err != nil {
			missing = append(missing, prog)
		}
	}
	return missing
}

func (g *game) cheer(s *step) {
	if s.kind == kindFlag {
		first := "  " + g.ui.col(ansiBold+ansiYellow, "🎉") + " "
		g.ui.proseWith(first, hangFor(first),
			span{"that's it — you found the flag! ", ansiBold}, span{s.done, ""})
		return
	}
	g.ok(s.done)
}

func (g *game) levelComplete(lv *level) {
	u := g.ui
	first := "  " + u.col(ansiBold+ansiYellow, "🎉") + " "
	u.lines("")
	if lv.isExtra() {
		u.proseWith(first, hangFor(first), span{"EXTRA COMPLETE", ansiBold})
	} else {
		u.proseWith(first, hangFor(first), span{fmt.Sprintf("LEVEL %d COMPLETE", lv.num), ansiBold})
	}
	u.proseWith("  "+u.col(ansiDim, "you just used: "), "  ",
		span{strings.Join(lv.used, " · "), ""})
	// A finished level is a finished level: the menu offers it with a ✅ next
	// time. Written here, at the moment the flag was found, so a run cut short
	// later still counts this one.
	if g.prog != nil {
		if lv.isExtra() {
			g.prog.markExtraDone()
		} else {
			g.prog.markDone(lv.num)
		}
		// Nothing is lost but the convenience of being remembered, and a
		// student mid-level does not need to hear about a failed write.
		_ = g.prog.save()
	}
	if lv.isExtra() {
		// The toolbox has no "next level": the menu is where it came from, and
		// where the ✅ it just earned will be waiting.
		u.prose("  ", "  ", "Press Enter for the menu (or Ctrl-D to stop here)")
		u.prompt()
		return
	}
	if lv.isLast() {
		return
	}
	u.prose("  ", "  ", fmt.Sprintf("Press Enter for Level %d (or Ctrl-D to stop here)", lv.num+1))
	u.prompt()
}

// waitBreak is the pause between levels: Enter continues, Ctrl-D (or `exit`)
// ends the run. A scripted run has nobody at the keyboard, so it continues.
func (g *game) waitBreak() bool {
	if !g.ui.tty {
		time.Sleep(breakPause)
		return true
	}
	for {
		switch ev := <-g.sess.events; ev.kind {
		case evEnter, evEOF:
			return true
		case evExit:
			return false // the student left bash at the break — so be it
		}
	}
}

// ── supervision ──────────────────────────────────────────────────────────────

// runStep shows one step and supervises it until it is done (or the session
// ends). It reports whether the step completed.
func (g *game) runStep(s *step) bool {
	st := &stepState{step: s, since: time.Now()}
	if err := plantAll(g.dir, s.plant); err != nil {
		fmt.Fprintln(os.Stderr, "tuxland:", err)
		return false
	}
	sym := "▶"
	if s.kind == kindFlag {
		sym = "🏁"
		g.sess.arm(s.token)
		defer g.sess.disarm()
	}
	g.card(sym, s.goal, s.note)
	// The exit-key beat is the one place the game reads the keyboard itself,
	// rather than watching bash: Ctrl-D must not reach the shell until the
	// student has said they mean it.
	if s.exitKey {
		if !g.runExitKeyBeat() {
			return false
		}
		// The answer was typed at the trap's prompt and is not echoed, so the
		// ✅ would land on the prompt's own line; start it on a fresh one.
		g.ui.lines("")
		g.cheer(s)
		return true
	}
	if !g.supervise(st) {
		return false
	}
	g.cheer(s)
	return true
}

// ── the exit key ─────────────────────────────────────────────────────────────

// trapPhase is where the Ctrl-D trap's conversation stands: the three things it
// can be waiting for and the one thing it can decide.
type trapPhase int

const (
	trapFirstD trapPhase = iota // waiting for Ctrl-D; nothing typed yet
	trapOther                   // a stray key arrived before Ctrl-D: one hint
	trapStay                    // Ctrl-D was pressed: wait for `y` or anything else
	trapLeave                   // `y`: the student really wants out
)

// trapKey is the trap's whole protocol as a pure function: where the conversation
// stands, one byte, where it stands next, and whether the game should be told.
// Pulling it out of the read loop is what makes the beat testable without a
// terminal.
//
// Before Ctrl-D any key is just someone poking the keyboard — worth one soft hint,
// and silence after that. Ctrl-D itself arms the question. Once armed, `y` means
// leave and every other key means stay.
func trapKey(phase trapPhase, c byte) (next trapPhase, tell bool) {
	switch phase {
	case trapFirstD:
		if c == 0x04 { // Ctrl-D
			return trapStay, true
		}
		return trapOther, true
	case trapOther:
		if c == 0x04 {
			return trapStay, true
		}
		return trapOther, false
	case trapStay:
		if c == 'y' || c == 'Y' {
			return trapLeave, true
		}
		return trapStay, true
	}
	return trapLeave, false
}

// runExitKeyBeat is the Ctrl-D beat: the game reads the keyboard itself, so the
// key that would normally end a shell never reaches bash unasked. The game asks
// first; only `y` lets the student out for real, and anything else finishes the
// beat and keeps them in the toolbox.
//
// The capture is armed for the beat and dropped on the way out, so bash gets the
// keyboard back the moment the beat ends.
func (g *game) runExitKeyBeat() bool {
	trap := make(chan trapPhase, 8)
	phase := trapFirstD
	g.kb.setCapture(func(b []byte) {
		for _, c := range b {
			next, tell := trapKey(phase, c)
			phase = next
			if tell {
				select {
				case trap <- next:
				default: // nobody is looking yet; the answer can wait
				}
			}
		}
	})
	defer g.kb.setCapture(nil)

	asked := false
	for {
		select {
		case p := <-trap:
			switch p {
			case trapOther:
				g.warn("not that one", "the key to try is Ctrl-D — hold Ctrl, press D")
				g.ui.prompt()
			case trapStay:
				if !asked {
					asked = true // this is Ctrl-D itself: ask the question
					g.ui.lines("")
					g.ui.proseWith("  ", "  ",
						span{"ok — a real bash would leave here.", ansiBold},
						span{" y to leave for real · anything else stays.", ""})
					g.ui.prompt()
					continue
				}
				return true // the answer was "stay": the beat is done
			case trapLeave:
				return false // `y`: the student really is leaving
			}
		case ev := <-g.sess.events:
			// The session can still end underneath us — the window closed, ssh
			// dropped, a scripted run ran out. Never leave the beat hanging.
			switch ev.kind {
			case evExit, evEOF:
				return false
			}
		}
	}
}

// stepState is how a step remembers the student: how many tries it took, and
// when we last had contact.
type stepState struct {
	step      *step
	tried     bool      // the student has submitted a line in this step
	presses   int       // submitted lines so far (info steps count their taps)
	fails     int       // failed submitted attempts
	nudged    int       // time-based nudges so far
	rescued   bool      // the way out has already been named on this card
	since     time.Time // last contact: progress, hint or nudge
	lastCheck time.Time
}

// rescue names the way out of a card that may not be satisfiable at all.
//
// The game only looks at the disk, which makes it trustworthy and blind: a
// student who has already moved or deleted the thing a card looks for can never
// satisfy the check, and a flag hunt says nothing on any attempt, so a deleted
// flag is silence. Broken and stuck look identical, and telling them apart would
// mean reading their input, which the game will not do — so it states the one
// thing true either way, once, and needs no diagnosis.
//
// Counted in submitted lines rather than seconds, deliberately: a student who
// thinks for ten minutes and then types once is not stuck and must not be told to
// give up. Six submissions of something that did not work is. Full rationale:
// DESIGN.md §3.
func (g *game) rescue(st *stepState) {
	if st.rescued || st.presses < stuckAfter {
		return
	}
	st.rescued = true
	g.warn("still nothing", "if you have deleted or renamed something this level needs, "+
		"`exit` and start it again — the playground is built fresh")
}

// hint cycles the step's hints so a stuck student always hears something new.
// It never contains the level's token — the flag is found, never handed over.
func (st *stepState) hint(n int) string {
	if len(st.step.hints) == 0 {
		return st.step.goal
	}
	return st.step.hints[n%len(st.step.hints)]
}

// What a submitted Enter means, once bash has stopped answering. The game can't
// tell *what* was typed, so it only ever reacts to the line boundary and to what
// the command left behind on disk.
const (
	nothing = iota
	readIt  // the command's output has landed — hand the line back, no verdict
	moveOn  // info step: the student has had their reading beat
	look    // task step: is the artifact there yet?
)

// supervise watches three things: the keyboard (an Enter was pressed), the
// clock (maybe check, maybe nudge), and the screen (the flag showed up).
func (g *game) supervise(st *stepState) bool {
	sess := g.sess
	tick := time.NewTicker(tickRate)
	defer tick.Stop()

	pending := nothing
	var at time.Time // when the line was submitted, for the fallback's hard cap

	// answer is what the game does with a submitted line once bash has finished
	// with it. The ✅ has to land *below* the command's output, not on top of it,
	// and both callers — the prompt marker and the fallback below — are after
	// every byte of that output.
	answer := func() bool {
		switch pending {
		case nothing:
			// bash came back to a prompt the game never asked for — a Ctrl-C, a
			// cleared screen. There is nothing to grade and nothing to say, but
			// the prompt line is the game's to draw, and without it the student
			// is left looking at ^C and a blank row while the shell quietly
			// waits for them.
			g.ui.prompt()
		case readIt:
			// Nothing to add — but bash drew no prompt either, so hand
			// the line back rather than leave a dead screen.
			//
			// This is also the flag hunt's only turn to speak. A hunt has no
			// check to fail, so a wrong guess is indistinguishable from a
			// right one until the token lands on the screen — which is right
			// while the word is still somewhere on the disk, and silence
			// forever if the student deleted it. Count the attempts here.
			g.rescue(st)
			g.ui.prompt()
		case moveOn:
			return true
		case look:
			done, spoke := g.tryTask(st, true)
			if done {
				return true
			}
			if !spoke {
				g.ui.prompt()
			}
		}
		pending = nothing
		return false
	}

	for {
		select {
		case ev := <-sess.events:
			switch ev.kind {
			case evExit:
				return false
			case evEOF:
				// A scripted run: nothing more will ever be typed. Give the step
				// a moment to settle, then end the game cleanly.
				if g.eof.IsZero() {
					g.eof = time.Now()
				}
			case evFlag:
				return true
			case evIdle:
				// bash is back at its own prompt, so it has nothing left to
				// say: answer now rather than sit out a guess.
				if answer() {
					return true
				}
			case evEnter:
				st.tried = true
				st.presses++
				at = time.Now()
				switch {
				case st.step.kind == kindInfo && (st.presses > 1 || st.step.noCmd):
					// The first Enter ran the command; this one is the "I've
					// read it" tap the card asks for. (A beat with nothing to
					// run needs only the tap.)
					pending = moveOn
				case st.step.kind == kindTask:
					pending = look
				default: // info (first Enter) and the flag hunt: nothing to say
					pending = readIt
				}
			}

		case now := <-tick.C:
			if !g.eof.IsZero() && now.Sub(g.eof) > eofGrace {
				return false
			}
			// The fallback, for a bash that will never print its prompt: the
			// prompt was taken away, or a child hangs without a word. Wait for
			// real silence rather than assume, so a merely slow command is
			// never talked over — its marker is on its way.
			if pending != nothing &&
				(now.Sub(sess.lastOutput()) >= quietGap || now.Sub(at) >= hardCap) {
				if answer() {
					return true
				}
			}
			// Backstop: a task whose command is still running, checked anyway.
			if st.tried && st.step.kind == kindTask && now.Sub(st.lastCheck) >= backstop {
				if done, _ := g.tryTask(st, false); done {
					return true
				}
			}
			// Nobody has typed anything for a while: a gentle nudge, with the
			// goal restated so nobody has to scroll back.
			if st.tried && now.Sub(st.since) >= nudgeEvery {
				phrase := nudgePhrases[st.nudged%len(nudgePhrases)]
				st.nudged++
				g.warn(phrase, st.step.goal)
				g.ui.prompt()
				st.since = now
			}
		}
	}
}

// tryTask looks at the real filesystem. driven says the check came from a
// submitted line, which is the only time the game speaks up: bash's own error
// message has already told the student what went wrong once. It reports whether
// the step is done, and whether the game said anything.
func (g *game) tryTask(st *stepState, driven bool) (done, spoke bool) {
	st.lastCheck = time.Now()
	if st.step.check(g.dir) {
		st.fails = 0
		st.since = time.Now()
		return true, false
	}
	if !driven {
		return false, false
	}
	st.fails++
	if st.fails >= hintAfter {
		// Two warnings, one prompt: the beat speaks twice and then hands the
		// terminal back once, which is what `spoke` tells the caller.
		g.warn("not there yet", st.hint(st.fails-hintAfter))
		g.rescue(st)
		g.ui.prompt()
		return false, true
	}
	return false, false
}
