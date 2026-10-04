package tuxland

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// The tests in ui_test.go draw ui's own writes into a buffer and ask what the
// screen would show. This file plays a real session instead: the real binary, a
// real pty, real raw mode, a real bash, the real relay — and then it renders the
// capture through the same model and reads the screen.
//
// That layer is where a card can be drawn perfectly and still land in the wrong
// place. Raw mode takes the kernel's newline translation away from the game's
// output, so a program that leaves the carriage return to the kernel stair-steps
// every line to the right while every byte stays correct — which is exactly what
// happened, and exactly what a check on the stream cannot see. The pty is also
// the only place the width is ever really read from a terminal, and the only
// place the shell's own output shares the screen with the game's.
//
// TUXLAND_WIDTH=80,60 plays at those widths instead of the default one, and
// TUXLAND_DUMP=1 prints the screen even when the test passes. Skipped under
// -short, off Linux, and without bash: this is the slow, real-thing test.

// TestTheStudentSeesTheSession plays the opening of level 2 — a reading card, a
// task with two misses and a hint, a step done, and the way out — and checks what
// the terminal would have on screen. Level 2 is the first level with tasks: level
// 1 is navigation only, so it has nothing to miss.
func TestTheStudentSeesTheSession(t *testing.T) {
	needRealSession(t)

	bin := buildGame(t)
	for _, cols := range widthsToPlay(t) {
		t.Run(fmt.Sprintf("%d columns", cols), func(t *testing.T) {
			s := playLevel2(t, bin, cols)
			t.Cleanup(func() {
				if t.Failed() || os.Getenv("TUXLAND_DUMP") != "" {
					s.show()
				}
			})

			// The level opens: its first card, with the note under it. Level 2
			// starts on a look-first beat, so the card asks for an `ls`.
			s.expectPrompt()
			s.expectRowStarting("  📁", "the level banner keeps its indent")
			s.expectRowStarting("  ▶", "the card goal keeps its indent")
			s.expectRowStarting("      ls", "the note hangs under the goal")

			// Ctrl-C is bash's own: the game is told only that the shell is back,
			// and the prompt line it draws is the whole of what the student has to
			// go on — without it the row after ^C stays blank while typing works.
			s.send("\x03")
			s.expectPrompt()

			// Each reading beat takes two Enters: the first runs the command, the
			// second is the "I've read it" tap the card asks for. Look, then read —
			// and the make-a-folder card is up after the second beat.
			s.enter("ls")
			s.expectPrompt()
			s.enter("")
			s.expectPrompt()
			s.expectRowStarting("      cat", "the read beat names the file")
			s.enter("cat notes/todo.txt")
			s.expectPrompt()
			s.enter("")
			s.expectPrompt()
			s.expectRowStarting("  ✅", "the reading beats are ticked off")
			s.expectRowStarting("      mkdir", "the next note hangs under its goal")

			// Two misses: bash says why, and then the game explains. The warning
			// is the longest line on the screen, so it is the one that wraps.
			s.enter("ls nope")
			s.expectPrompt()
			s.enter("ls nope")
			s.expectPrompt()
			hint := s.expectRowStarting("  ⚠", "the hint keeps its indent")
			if !strings.Contains(hint, "not there yet") {
				t.Errorf("the miss line does not say what happened: %q", hint)
			}
			// A wrap must land under the hang, not under the line's own length.
			if next := s.rowAfter("  ⚠"); next != "" {
				if !strings.HasPrefix(next, "    ") {
					t.Errorf("the wrapped hint does not hang under its prefix: %q", next)
				}
			}

			// The step, done: the game says so and moves on to the next card.
			s.enter("mkdir done")
			s.expectPrompt()
			s.expectRowStarting("  ✅", "the done line keeps its indent")

			// The way out: the bye's commands are glued to their words, so no
			// row may begin with a flag.
			s.send("\x04")
			s.expectRowStarting("  🐧 leaving the playground", "the bye keeps its indent")
			m := s.rendered()
			for _, row := range m.rows() {
				if strings.HasPrefix(strings.TrimSpace(row), "-") {
					t.Errorf("a command was split from its word: %q", row)
				}
			}

			// The one thing every screen must do, checked on the real stream:
			// every line begins at the left margin.
			for i, col := range m.starts {
				if col != 0 {
					t.Errorf("row %d begins at column %d, not 0 — the terminal slid it right", i+1, col)
				}
			}
		})
	}
}

// TestTheGameWaitsForBashToFinish checks the order the whole design hangs on: the
// game speaks *after* the shell, never into the middle of its answer. It is the
// one thing a stream check cannot see and a timer cannot guarantee — a fixed wait
// that is long enough for `ls` is too short for a command that takes its time,
// and short enough for a slow one is dead air on every other command.
//
// So the test gives bash something slow and silent-ish to do, and asks where the
// game's prompt landed: below the output, always. A command that takes most of a
// second is also the case the old fixed wait got wrong, so the test is a real
// guard, not a restatement of the current code.
func TestTheGameWaitsForBashToFinish(t *testing.T) {
	needRealSession(t)
	bin := buildGame(t)

	for _, cols := range widthsToPlay(t) {
		t.Run(fmt.Sprintf("%d columns", cols), func(t *testing.T) {
			s := playLevel1(t, bin, cols)
			t.Cleanup(func() {
				if t.Failed() || os.Getenv("TUXLAND_DUMP") != "" {
					s.show()
				}
			})
			s.expectPrompt() // the opening card, and an invitation to type

			// A command that says something, but not for a while — long enough
			// that any fixed "wait and see" would talk over it. That is the
			// point: the game must be waiting on the shell, not on a number.
			s.enter("sleep 0.7; echo tuxland-slow-output")
			s.expectPrompt() // the game hands the line back

			// The output must be a row of its own, and the prompt must be
			// below it. Both halves can be perfect alone: the game answering
			// first, or answering into the middle of the output, both leave a
			// correct byte stream and a screen that reads as nonsense.
			rows := s.screen()
			said := rowIndexOf(rows, "tuxland-slow-output")
			prompt := lastRowIndexOf(rows, "tuxland$")
			switch {
			case said < 0:
				t.Errorf("bash's output is not a row of its own — the game drew over it: %q", rows)
			case prompt < 0:
				t.Errorf("the game never drew its prompt: %q", rows)
			case prompt < said:
				t.Errorf("the game answered before the shell had finished: output at row %d, "+
					"its prompt at row %d", said+1, prompt+1)
			}
		})
	}
}

// expectMenu waits for the menu's prompt line to read the given level, and
// checks that line is the only copy of itself on screen and the last thing
// there. A repaint that left the tail of a longer name behind would show up as
// a second row, or as the row with something extra on the end of it — and the
// whole point of the repaint is that the screen holds exactly one of these,
// always, while the menu is the screen.
func (s *playtest) expectMenu(want int) string {
	s.t.Helper()
	prefix := fmt.Sprintf("tuxland ▶ %d", want)
	deadline := time.Now().Add(20 * time.Second)
	for {
		rows := s.screen()
		at := -1
		for i, row := range rows {
			if !strings.HasPrefix(row, "tuxland ") {
				continue // the level prompt is "tuxland$", which is not this line
			}
			if at >= 0 {
				s.show()
				s.t.Fatalf("the menu drew two prompt lines and left both: %q", rows)
			}
			at = i
		}
		if at >= 0 {
			if at != len(rows)-1 {
				s.show()
				s.t.Fatalf("something was drawn under the menu's prompt line: %q", rows)
			}
			if strings.HasPrefix(rows[at], prefix) {
				return rows[at]
			}
		}
		if time.Now().After(deadline) {
			s.show()
			s.t.Fatalf("the menu's prompt never read %q on this screen", prefix)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestTheMenuPicksTheLevel drives the opening screen the way a student does: no
// command-line flag at all, a real pty, real arrow keys and a real digit typed at
// a real prompt. Both ways of choosing have to land on the level they name, and
// the prompt line has to be the only copy of itself on screen — a repaint that
// left the tail of a longer name behind would show up as a second row, or as a
// row with something extra on the end of it.
func TestTheMenuPicksTheLevel(t *testing.T) {
	needRealSession(t)
	bin := buildGame(t)

	for _, tc := range []struct {
		name  string
		keys  string // what the student types, in one burst like a confident thumb
		want  int    // where the selection should be after keys
		enter bool   // keys end in Enter, so a level opens
		emoji string // the banner of the level that opens
	}{
		{"just press enter", "\r", 1, true, "🧭"},
		{"arrow down twice, still looking", "\x1b[B\x1b[B", 3, false, ""},
		{"arrow down twice, then go", "\x1b[B\x1b[B\r", 3, true, "🔀"},
		{"arrow down, then back up", "\x1b[B\x1b[B\x1b[A", 2, false, ""},
		{"up at the top stays put", "\x1b[A", 1, false, ""},
		{"type the number, still looking", "5", 5, false, ""},
		{"type the number, then go", "5\r", 5, true, "🛠️"},
		{"type the last number, then go", "6\r", 6, true, "🔐"},
		{"arrow, then type over it", "\x1b[B4", 4, false, ""},
		{"arrow, then type over it and go", "\x1b[B4\r", 4, true, "🔍"},
	} {
		for _, cols := range widthsToPlay(t) {
			t.Run(fmt.Sprintf("%s at %d columns", tc.name, cols), func(t *testing.T) {
				s := playGame(t, bin, cols, nil, t.TempDir())
				t.Cleanup(func() {
					if t.Failed() || os.Getenv("TUXLAND_DUMP") != "" {
						s.show()
					}
				})

				// The menu opens on the level a first-time student would be
				// offered, and it is whole — that is what the student reads.
				s.expectMenu(1)
				s.send(tc.keys)
				if tc.enter {
					// They went: the line they committed to — repainted even
					// when the whole burst arrived in one read — and beneath it
					// the level that opens.
					line := s.expectRowStarting(fmt.Sprintf("tuxland ▶ %d", tc.want), "the menu's committed line")
					full := fmt.Sprintf("tuxland ▶ %d %s", tc.want, levelByNum(tc.want).name)
					if widthOf(full)+1 <= cols && !strings.Contains(line, levelByNum(tc.want).name) {
						t.Errorf("the menu line does not name the level: %q", line)
					}
					got := s.expectRowStarting("  "+tc.emoji, "the chosen level's banner")
					if !strings.Contains(got, fmt.Sprintf("LEVEL %d", tc.want)) {
						t.Errorf("the banner says %q, want level %d", got, tc.want)
					}
					return
				}
				// Still deciding: the line moved to their choice, and the menu
				// is still the last thing on the screen.
				line := s.expectMenu(tc.want)
				full := fmt.Sprintf("tuxland ▶ %d %s", tc.want, levelByNum(tc.want).name)
				if widthOf(full)+1 <= cols && !strings.Contains(line, levelByNum(tc.want).name) {
					t.Errorf("the menu line does not name the level: %q", line)
				}
			})
		}
	}
}

// TestTheMenuRemembersWhatYouFinished is the same screen with a student who has
// been here before: the finished levels carry a ✅, the count is spelled out, and
// the cursor starts on the first one that is left rather than on level 1.
func TestTheMenuRemembersWhatYouFinished(t *testing.T) {
	needRealSession(t)
	bin := buildGame(t)

	for _, tc := range []struct {
		name    string
		done    []int
		extra   bool   // the habits toolbox finished too
		says    string // a phrase the progress line has to contain
		offered int    // where the cursor starts
	}{
		{"none yet", nil, false, "no levels finished yet", 1},
		{"two of six", []int{1, 2}, false, "2 of 6 levels done · next up: level 3", 3},
		{"a gap at the front", []int{2, 3}, false, "2 of 6 levels done", 1},
		{"all six", []int{1, 2, 3, 4, 5, 6}, false, "6 of 6 levels done · every level open", 1},
		{"the toolbox done too", []int{1}, true, "1 of 6 levels done · next up: level 2", 2},
	} {
		for _, cols := range widthsToPlay(t) {
			t.Run(fmt.Sprintf("%s at %d columns", tc.name, cols), func(t *testing.T) {
				lab := t.TempDir()
				// Written before the game looks — the record of a past run, in
				// the place the game itself would have written it. The lab has
				// to be pointed at the same folder in this process and the
				// child's, or the file goes somewhere nothing reads.
				t.Setenv("LINUXLAB_ROOT", lab)
				past := progressOf(tc.done...)
				if tc.extra {
					past.markExtraDone()
				}
				if err := past.save(); err != nil {
					t.Fatal(err)
				}
				s := playGame(t, bin, cols, nil, lab)
				t.Cleanup(func() {
					if t.Failed() || os.Getenv("TUXLAND_DUMP") != "" {
						s.show()
					}
				})

				// The menu is the gate: when its line is up, the whole screen
				// is drawn, which is also the moment the checks below can trust
				// what they read. Expecting the offered level doubles as the
				// wait and as the integration check.
				s.expectMenu(tc.offered)
				// The count is spelled out on the screen...
				if got := s.rendered().text(); !strings.Contains(got, tc.says) {
					t.Errorf("the screen does not say %q:\n%s", tc.says, got)
				}
				// ...the finished levels carry a ✅ and only those — and the
				// numbers still line up, which is the whole reason the mark is a
				// fixed three-column gutter and not glued to the number.
				rows := s.screen()
				for n := 1; n <= NumLevels(); n++ {
					done := false
					for _, d := range tc.done {
						if d == n {
							done = true
						}
					}
					if !done && rowIndexOf(rows, fmt.Sprintf("  ✅ %d  ", n)) >= 0 {
						t.Errorf("level %d carries a ✅ but should not", n)
					}
					if done && rowIndexOf(rows, fmt.Sprintf("  ✅ %d  ", n)) < 0 {
						t.Errorf("level %d has no ✅ but should", n)
					}
				}
				// The toolbox is ticked in its own column — it has no number —
				// and only when it was actually finished.
				if tc.extra && rowIndexOf(rows, "  ✅ ✦  ") < 0 {
					t.Errorf("the toolbox has no ✅ but should: %q", rows)
				}
				if !tc.extra && rowIndexOf(rows, "  ✅ ✦  ") >= 0 {
					t.Errorf("the toolbox carries a ✅ but should not: %q", rows)
				}
			})
		}
	}
}

// TestTheExtraTrapsTheExitKey is the one beat where the game reads the keyboard
// itself. The toolbox is reached from the menu only, and its last step is Ctrl-D —
// the key that would normally end a shell. The game asks first, so a student can
// try the key without losing their place, and only `y` really leaves.
func TestTheExtraTrapsTheExitKey(t *testing.T) {
	needRealSession(t)
	bin := buildGame(t)

	for _, cols := range widthsToPlay(t) {
		t.Run(fmt.Sprintf("staying at %d columns", cols), func(t *testing.T) {
			s := playExtra(t, bin, cols)
			t.Cleanup(func() {
				if t.Failed() || os.Getenv("TUXLAND_DUMP") != "" {
					s.show()
				}
			})

			// Ctrl-D: bash would leave here. The game says so and waits for a
			// real answer rather than passing the key through.
			s.send("\x04")
			confirm := s.expectRowStarting("  ok — a real bash would leave here.", "the trap asks before exiting")
			// The sentence wraps on a narrow terminal; rebuild it (collapsing the
			// hang's spaces) and check the bit that says how to actually leave.
			line := strings.Join(strings.Fields(confirm+" "+s.rowAfter("  ok — a real bash would leave here.")), " ")
			if !strings.Contains(line, "y to leave for real") {
				t.Errorf("the trap does not say how to leave: %q", line)
			}

			// Anything but `y` stays: the beat finishes, the toolbox is ticked
			// off, and the menu comes back with its ✅.
			s.send("x")
			s.expectRowStarting("  ✅ the exit key is yours now.", "the trap finishes when the student stays")
			s.expectRowStarting("  🎉 EXTRA COMPLETE", "the toolbox is ticked off")
			s.enter("")
			// The menu is back — the toolbox row carries the ✅ it just earned.
			// (Not expectMenu: the first menu's committed line is still on the
			// screen above this one, and it is what that helper would find.)
			s.expectRowStarting("  ✅ ✦  ", "the toolbox earns its ✅ on the returning menu")
			if text := s.rendered().text(); !strings.Contains(text, "tuxland ▶ 1 navigation") {
				t.Errorf("the menu did not come back on the first level:\n%s", text)
			}
		})

		t.Run(fmt.Sprintf("leaving at %d columns", cols), func(t *testing.T) {
			s := playExtra(t, bin, cols)
			t.Cleanup(func() {
				if t.Failed() || os.Getenv("TUXLAND_DUMP") != "" {
					s.show()
				}
			})

			// Ctrl-D then `y`: this time the student means it, and the game lets
			// the key through — the run ends with the bye, not with a shell that
			// vanished under the screen.
			s.send("\x04")
			s.expectRowStarting("  ok — a real bash would leave here.", "the trap asks before exiting")
			s.send("y")
			s.expectRowStarting("  🐧 leaving the playground", "`y` really leaves")
		})
	}
}

// playExtra gets the game to the toolbox's Ctrl-D card: the menu (no flag), one
// down per numbered level to reach the ✦ row, Enter, and then the reading cards
// before it. The game never checks an info step, so an empty line is a fair way
// to turn each page — and the prompt is consumed after every send so a pair of
// taps can't outrun the step they belong to.
//
// The downs are built from NumLevels() rather than counted out: a level added
// later moves the ✦ row down, and a hard-coded five would quietly walk into
// level 6 and hang on a `chmod` card instead.
func playExtra(t *testing.T, bin string, cols int) *playtest {
	t.Helper()
	s := playGame(t, bin, cols, nil, t.TempDir())
	s.expectMenu(1)
	s.send(strings.Repeat("\x1b[B", NumLevels()) + "\r")
	s.expectRowStarting("  🧰", "the toolbox banner")
	s.expectPrompt() // the first card's own prompt
	// The reading cards before the Ctrl-D one: man, --help, Tab, ↑, clear, Ctrl-C.
	for range 6 {
		s.enter("")      // run the line; an info step reads nothing into it
		s.expectPrompt() // the reading prompt
		s.enter("")      // the "I've read it" tap
		s.expectPrompt() // the next card — the Ctrl-D one, after the fifth
	}
	return s
}

// needRealSession skips the tests that need what only a real session has: a pty,
// raw mode and bash. -short turns them off; the layout tests in ui_test.go and
// the content tests in levels_test.go stay on.
func needRealSession(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("a session needs a pty and a real shell: Linux only")
	}
	if testing.Short() {
		t.Skip("-short: skipping the real-session test")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on PATH")
	}
}

// widthsToPlay is the widths to run a session at: one by default, because the
// stair-step is not width-dependent and the narrow one is the one that wraps
// (the layout's own widths are covered exhaustively in ui_test.go), and whatever
// TUXLAND_WIDTH asks for otherwise.
func widthsToPlay(t *testing.T) []int {
	t.Helper()
	spec := os.Getenv("TUXLAND_WIDTH")
	if spec == "" {
		return []int{60}
	}
	var out []int
	for _, f := range strings.Split(spec, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 20 {
			t.Fatalf("TUXLAND_WIDTH=%q: not a width of at least 20 columns", spec)
		}
		out = append(out, n)
	}
	return out
}

// buildGame compiles the real binary, so the test drives exactly what a player
// runs — raw mode, the signal handler, the pty, all of it. The program lives two
// directories up (this package is the library underneath it), so it is built by
// walking to the module root rather than by naming a package: a bare `tuxland`
// only resolved while the module was called `tuxland`, and `.` here would build
// this library instead of the command.
func buildGame(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "tuxland")
	root := filepath.Join("..", "..")
	if out, err := runFor(exec.Command("go", "build", "-o", bin, root), 90*time.Second); err != nil {
		t.Fatalf("cannot build the game: %v\n%s", err, out)
	}
	return bin
}

// runFor runs a command to completion, or kills it and says so, rather than
// hanging the whole test run on a machine that is busy.
func runFor(cmd *exec.Cmd, d time.Duration) (string, error) {
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return out.String(), err
	case <-time.After(d):
		_ = cmd.Process.Kill()
		return out.String(), fmt.Errorf("no answer after %s", d)
	}
}

// playtest is a running game, read as a stream of what it wrote. (Not the
// production session: that is one bash, this is the whole thing.)
type playtest struct {
	t    *testing.T
	ptmx *os.File
	raw  bytes.Buffer // the stream, as the terminal receives it
	mu   sync.Mutex
	seen int // how far we have read when waiting for the next prompt
	cols int
}

func playLevel1(t *testing.T, bin string, cols int) *playtest {
	t.Helper()
	return playGame(t, bin, cols, []string{"-level", "1"}, t.TempDir())
}

func playLevel2(t *testing.T, bin string, cols int) *playtest {
	t.Helper()
	return playGame(t, bin, cols, []string{"-level", "2"}, t.TempDir())
}

// playGame starts the real binary on a pty of a pinned width, with its own
// playground. args is what the student (or the test) typed on the command line;
// lab is the $LINUXLAB_ROOT it gets, so a test can seed it — with a progress file,
// say — before the game looks.
func playGame(t *testing.T, bin string, cols int, args []string, lab string) *playtest {
	t.Helper()
	s := &playtest{t: t, cols: cols}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"LINUXLAB_ROOT="+lab, // its own playground, thrown away after
		"TERM=xterm-256color",
	)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: 24})
	if err != nil {
		t.Fatalf("cannot start the game on a %d-column pty: %v", cols, err)
	}
	s.ptmx = ptmx
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		// A level may seal a folder — level 6 does, at 0600, and that is the
		// point of it. TempDir's cleanup cannot delete a folder without owner
		// `x`, and it is registered before this one, so it runs *after* us and
		// would fail on a tree the game built correctly. Lift the seals first.
		unseal(lab)
	})
	go s.read(ptmx)
	return s
}

// read drains the pty — a full buffer would block the game in the middle of a
// card — and keeps every byte, because the screen is rendered from the stream
// at the end rather than from the pieces as they arrive.
func (s *playtest) read(ptmx *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.raw.Write(buf[:n])
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// waitFor blocks until want — a plain, escape-free string — has been written
// after the last wait.
func (s *playtest) waitFor(want string, d time.Duration) string {
	s.t.Helper()
	return s.waitFrom(s.cursor(), want, d)
}

// cursor is how far into the stream we have read. waitFrom takes one of these when
// the wait has to be about bytes written after a known point, which is the only way
// to tell one prompt from the next.
func (s *playtest) cursor() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen
}

// mark is a point in the stream for a later wait to start from. It is the
// difference between waiting for "the prompt" and waiting for "the prompt that
// answers this line": a card draws its own prompt before the student types, the
// shell draws another when the command is done, so a wait with no mark can be
// satisfied by the prompt that was already on screen and the next keystroke goes
// out before the shell is listening for it.
func (s *playtest) mark() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	// In stripped coordinates, which is what every wait indexes into: the raw
	// length counts escape sequences the search never sees, and marking on it
	// overshoots the text by however many the game had drawn.
	return len(stripANSI(s.raw.String()))
}

// waitFrom blocks until want has been written after from.
func (s *playtest) waitFrom(from int, want string, d time.Duration) string {
	s.t.Helper()
	if got := s.waitAnyFrom(from, d, want); got != "" {
		return got
	}
	return ""
}

// waitAnyFrom blocks until whichever of wants turns up first *at or after* from,
// and says which. It searches from from rather than from wherever the last wait
// stopped, because the two are not the same thing: a mark says "after this line",
// and a check that looks at a whole run has to be able to look behind the cursor
// again.
//
// It is also the honest way to wait on something with two legitimate endings —
// name both and let the game choose, rather than guessing which one this takes.
func (s *playtest) waitAnyFrom(from int, d time.Duration, wants ...string) string {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for {
		s.mu.Lock()
		text := stripANSI(s.raw.String())
		if from > len(text) {
			from = len(text)
		}
		for _, want := range wants {
			if at := strings.Index(text[from:], want); at >= 0 {
				s.seen = from + at + len(want)
				s.mu.Unlock()
				return want
			}
		}
		s.mu.Unlock()
		if time.Now().After(deadline) {
			s.show()
			s.t.Fatalf("nothing in the stream within %s mentions any of %q", d, wants)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// expectPrompt waits for the game's own prompt — "tuxland$ " in the student's
// green and bold. It is drawn at the end of every block that precedes typing, so
// it is a real handshake, not a guess at how long the game takes.
func (s *playtest) expectPrompt() {
	s.waitFor("tuxland$", 20*time.Second)
}

// expectEcho waits for the line just typed to come back on screen, which is the
// shell saying it has the line. Waiting for that before waiting for the prompt is
// what keeps a replay honest: the prompt is only worth having once the command
// behind it has actually been submitted.
func (s *playtest) expectEcho(cmd string) {
	s.t.Helper()
	if cmd == "" {
		return // a bare Enter echoes nothing
	}
	s.waitFor(cmd, 20*time.Second)
}

// expectAnswer waits for the game to be ready for the next line: its prompt, or the
// end of the level. A flag hunt takes the level down the instant the word reaches
// the screen, so the last command of a level can leave no prompt behind — and a
// replay that waited for one there would hang on the one card that worked.
func (s *playtest) expectAnswer(from, num int) {
	s.t.Helper()
	s.waitAnyFrom(from, 30*time.Second, "tuxland$", fmt.Sprintf("LEVEL %d COMPLETE", num))
}

// expectLevelComplete waits for the banner a finished level draws, counting from
// where the level started rather than from the last thing waited for.
func (s *playtest) expectLevelComplete(from, num int) {
	s.waitFrom(from, fmt.Sprintf("LEVEL %d COMPLETE", num), 20*time.Second)
}

// expectCard waits for the card whose goal contains want. Matching on a fragment
// of the goal, anywhere on the screen, is what lets a replay name cards the way
// the card does rather than by an index that every edit would move.
//
// The number is only in the failure message: a replay that fails is much easier to
// read as "card 4 wanted X" than as whatever the screen happened to be showing.
func (s *playtest) expectCard(want string, n int) {
	s.t.Helper()
	if want == "" {
		s.t.Fatalf("replay step %d names no card to wait for", n+1)
	}
	needle := asShown(want)
	deadline := time.Now().Add(20 * time.Second)
	for {
		if strings.Contains(asShown(strings.Join(s.screen(), " ")), needle) {
			return
		}
		if time.Now().After(deadline) {
			s.show()
			s.t.Fatalf("replay step %d: no card on screen mentions %q", n+1, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// asShown is a card's goal as the screen reads it: the code backticks and the
// emphasis stars are drawn as text, not as marks, and a goal is one sentence to the
// student and two or three rows on the screen. Applying it to both the goal and
// the fragment is what lets a replay name a card by a phrase of it without caring
// where the wrap fell or how the card was marked up.
func asShown(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("`", "", "*", "").Replace(s)), " ")
}

// waitForAny waits for whichever of wants turns up first, and reports which. It is
// the honest way to wait on something with two legitimate endings: name both and
// let the game choose, rather than guessing which one this level will take.
func (s *playtest) waitForAny(d time.Duration, wants ...string) string {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for {
		s.mu.Lock()
		text := stripANSI(s.raw.String())
		for _, want := range wants {
			if strings.Contains(text[s.seen:], want) {
				s.seen += strings.Index(text[s.seen:], want) + len(want)
				s.mu.Unlock()
				return want
			}
		}
		s.mu.Unlock()
		if time.Now().After(deadline) {
			s.show()
			s.t.Fatalf("nothing in the stream within %s mentions any of %q", d, wants)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// send types at the terminal, byte for byte, the way a student would: a control
// key is a key.
func (s *playtest) send(keys string) {
	s.t.Helper()
	if _, err := s.ptmx.WriteString(keys); err != nil {
		s.t.Fatalf("cannot type %q: %v", keys, err)
	}
}

// enter types a command and presses Enter.
func (s *playtest) enter(cmd string) { s.send(cmd + "\n") }

// expectRowStarting waits for the screen to hold a row beginning with prefix, and
// returns that row. It looks at everything the game has drawn so far, not only
// what came after the last wait: what the student sees is the whole screen, and a
// card is drawn before the prompt that invites the typing.
func (s *playtest) expectRowStarting(prefix, what string) string {
	s.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		for _, row := range s.screen() {
			if strings.HasPrefix(row, prefix) {
				return row
			}
		}
		if time.Now().After(deadline) {
			s.show()
			s.t.Errorf("%s: no row begins with %q", what, prefix)
			return ""
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// rowAfter is the next row with text on it after the row beginning with prefix —
// where a wrapped line continues. A prompt is not a continuation: it is how the
// game says the hint fitted on one line.
func (s *playtest) rowAfter(prefix string) string {
	rows := s.screen()
	for i, row := range rows {
		if !strings.HasPrefix(row, prefix) {
			continue
		}
		for _, next := range rows[i+1:] {
			switch {
			case next == "":
				continue
			case strings.HasPrefix(next, "tuxland$"):
				return ""
			default:
				return next
			}
		}
	}
	return ""
}

// rowIndexOf is where prefix first appears on the screen, or -1. Order is the
// point: what the student reads top to bottom is what the game wrote in what
// order, and a prompt drawn before a command's output is a screen that makes no
// sense however correct each half is on its own.
func rowIndexOf(rows []string, prefix string) int {
	for i, row := range rows {
		if strings.HasPrefix(row, prefix) {
			return i
		}
	}
	return -1
}

// lastRowIndexOf is the same, for the *newest* row with that prefix: a session
// draws its prompt again after every block, and the one that matters is the last.
func lastRowIndexOf(rows []string, prefix string) int {
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.HasPrefix(rows[i], prefix) {
			return i
		}
	}
	return -1
}

// rendered is the screen so far, the way the terminal would show it: everything
// the game has written, fed through the model in one go, because a screen is not
// a sequence of writes.
func (s *playtest) rendered() *screenModel {
	s.mu.Lock()
	raw := s.raw.String()
	s.mu.Unlock()
	m := newScreenModel(s.cols)
	m.feed(raw)
	return m
}

// screen is the text on each row of the rendered screen.
func (s *playtest) screen() []string { return s.rendered().rows() }

// show prints the screen with a ruler, so a failure can be read, and the raw
// stream under it. The screen is what the student sees and the stream is what the
// game actually wrote, and a failure is usually the difference between the two —
// a card that was drawn and then scrolled away, or a keystroke that arrived before
// the prompt it was meant to follow. Always on failure, and on success when
// TUXLAND_DUMP asks for it.
func (s *playtest) show() {
	rows := s.screen()
	s.t.Logf("the %d-column screen the student sees:", s.cols)
	s.t.Logf("     %s", strings.Repeat("0123456789", 1+(s.cols/10)))
	for i, row := range rows {
		if row != "" {
			s.t.Logf("%4d | %s", i+1, row)
		}
	}
	s.mu.Lock()
	stream := stripANSI(s.raw.String())
	s.mu.Unlock()
	s.t.Logf("the stream, in order:")
	for i, line := range strings.Split(strings.ReplaceAll(stream, "\r", "\n"), "\n") {
		if line != "" {
			s.t.Logf("%4d > %s", i+1, line)
		}
	}
}
