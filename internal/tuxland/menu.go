package tuxland

import (
	"fmt"
	"strings"
)

// The opening screen is a real menu, but a small one. The level list is drawn
// once — it cannot change while nobody is playing — and the *choice* lives on
// the prompt line under it, repainted in place as the student moves:
//
//	tuxland ▶ 4 executables
//
// So there is no full-screen repaint, no second escape sequence, and nothing that
// goes wrong when the window is too short for the list. Choosing is also real
// typing in the game's own voice: the game never parses what a word means, but a
// number and an arrow key are unambiguous, so both are accepted.

// menuChoice is where a read's worth of keystrokes leaves the selection. done
// means the student pressed Enter; quit that they left (Ctrl-C or Ctrl-D, which in
// raw mode arrive as plain bytes rather than a signal).
type menuChoice struct {
	level int
	done  bool
	quit  bool
}

// menuKey applies one read's worth of keystrokes to the current selection, and
// hands back whatever was left over.
//
// It is a pure function of the bytes on purpose: the terminal delivers an escape
// sequence in whatever chunks it likes, and an arrow key split across two reads
// must move the selection once — not twice, and not not at all.
func menuKey(choice int, keys []byte) (menuChoice, []byte) {
	m := menuChoice{level: choice}
	for i := 0; i < len(keys); {
		switch k := keys[i]; {
		case k == '\r' || k == '\n':
			m.done = true
			return m, nil
		case k == 0x03 || k == 0x04: // Ctrl-C, Ctrl-D: the way out of every screen here
			m.quit = true
			return m, nil
		case k >= '1' && k <= '9': // type the number and go
			// Numbers only reach the numbered levels: the toolbox has no
			// number, so there is nothing for the digit past the last one to
			// mean.
			if int(k-'0') <= NumLevels() {
				m.level = int(k - '0')
			}
			i++
		case k == 0x1b:
			n := arrowLen(keys[i:])
			switch {
			case n == 0:
				return m, keys[i:] // half an escape sequence: wait for the rest
			case n == 1:
				i++ // a lone Escape, or a sequence we do not use
			case keys[i+n-1] == 'A':
				if m.level > 1 {
					m.level--
				}
				i += n
			case keys[i+n-1] == 'B':
				if m.level < len(levels) {
					m.level++
				}
				i += n
			default:
				i += n
			}
		default:
			i++ // anything else: not ours to read
		}
	}
	return m, nil
}

// arrowLen is how long the escape sequence starting at keys[0] is: `\x1b[A` up
// and `\x1b[B` down, the two-byte forms a terminal in application-cursor mode
// sends (`\x1bOA`), and the longer parameterised ones (`\x1b[1;5A`, the `\x1b[200~`
// a paste sends) which we swallow whole so their parameters are never read as keys.
//
// One means the introducer is not ours to interpret — swallow just that byte and
// let the rest be ordinary keys. Zero means the sequence is incomplete, and the
// caller holds it for the next read: a lone Escape at the end of a read is far
// more often the first byte of an arrow key than a student pressing Escape.
//
// A sequence that can never complete — an Escape, a stray introducer, a control
// character where a parameter belongs — counts as one, deliberately. Holding
// those would swallow whatever came next, and a student who pasted something odd
// would find they could not press Enter to get out. This way the worst a
// malformed sequence costs is the one keystroke that follows it.
func arrowLen(keys []byte) int {
	if len(keys) < 2 {
		return 0
	}
	if keys[1] != '[' && keys[1] != 'O' {
		return 1
	}
	for i := 2; i < len(keys); i++ {
		switch b := keys[i]; {
		case b >= 0x20 && b <= 0x3f: // a parameter or intermediate byte: keep reading
		case b >= 0x40 && b <= 0x7e: // a final byte: the sequence ends here
			return i + 1
		default:
			return 1
		}
	}
	return 0
}

// askLevel is the menu loop: the student's own keystrokes, one repaint per
// change, until they press Enter. It returns the level position to start at and
// whether to start at all — false when nobody is there to press anything (a pipe,
// a piped test script) or when the student walked out with Ctrl-D.
//
// The keys come through the shared keyboard (kb), the same reader the levels use,
// so there is only ever one goroutine on stdin — the menu can come back as often
// as the toolbox sends it there. The prompt line is drawn here, not by kickoff,
// because the repaints are this loop's business: kickoff puts up everything above
// it, this owns the last line until the game moves on.
func askLevel(u *ui, kb *keyboard, p progress) (int, bool) {
	choice := p.next()

	// No terminal, no menu: a scripted run cannot answer a question, so it is
	// offered the same level a returning student would be and moves straight on.
	if !u.tty {
		u.menuPrompt(choice, 0)
		u.lines("")
		return choice, true
	}

	// Raw mode is already on (the keyboard set it up), so the echo is ours: the
	// terminal must not also print the digit the student typed, and the arrow key
	// must arrive as a sequence rather than be eaten by the line discipline.
	keys := make(chan []byte, 16)
	kb.setCapture(func(b []byte) {
		select {
		case keys <- append([]byte(nil), b...):
		default: // a burst faster than the repaint: the newest keys are the ones that count
		}
	})
	defer kb.setCapture(nil)

	widest := u.menuPrompt(choice, 0)
	var pending []byte
	for b := range keys {
		// Copied rather than aliased: pending may point into an older read.
		k := append(append([]byte(nil), pending...), b...)
		m, leftover := menuKey(choice, k)
		choice, pending = m.level, append([]byte(nil), leftover...)
		// Every read repaints, even a committing one: a burst of keys that lands
		// in a single read — the terminal handed over ↓ ↓ Enter together — has to
		// end with the level it is committing to on the screen, not the level the
		// read began at.
		widest = u.menuPrompt(choice, widest)
		if m.done {
			u.lines("")
			return choice, true
		}
		if m.quit {
			u.lines("")
			u.proseWith("  ", "  ",
				span{"🐧 leaving the menu", ansiBold},
				span{" — your progress is saved. come back any time.", ansiDim})
			u.lines("")
			return choice, false
		}
	}
	return choice, false // the keyboard closed under us
}

// menuPrompt draws the selection: the game asking which level, in the game's own
// prompt so the screen reads as one thing. It returns how many columns the line
// took, which is what the next repaint has to pad over — there is no erase
// sequence here, on purpose, and a shorter line has to cover the longer one it
// replaces or its tail stays on screen.
//
// widest is 0 on the first draw, which is also the only time no carriage return
// is written: nothing is on the line to cover yet, and a pipe gets plain text.
func (u *ui) menuPrompt(choice, widest int) int {
	first := widest == 0 // nothing on the line yet, so nothing to cover
	lv := levelAt(choice)
	head := u.col(ansiGreen+ansiBold, "tuxland ")
	mark := u.col(ansiCyan, "▶ "+fmt.Sprint(choice))
	if lv.isExtra() {
		// The toolbox has no number: name it instead of printing one.
		mark = u.col(ansiCyan, "▶ extra")
	}
	line := head + mark + " " + u.col(ansiBold, lv.name)
	// A narrow window gets the number on its own: the name is the first thing to
	// go, and a prompt that wrapped would be repainted across two rows. The line
	// is measured stripped — the colour escapes take no columns, and counting
	// their bytes would drop the name on a perfectly wide-enough window.
	lineW := widthOf(stripANSI(line))
	if lineW+1 > u.width() {
		line, lineW = head+mark, widthOf(stripANSI(head+mark))
	}
	pad := lineW
	if first {
		widest = pad
	}
	if pad < widest { // cover the tail of a longer line we are replacing
		pad = widest
	}
	// Never pad into the last column: a full-width line and the next keystroke's
	// repaint is a wrap the model (and the student) did not ask for.
	if room := u.width() - 1; pad > room {
		pad = room
	}
	if !first {
		line = "\r" + line
	}
	u.write(padANSI(line, pad))
	return pad
}

// padANSI pads an already-styled string to n visible columns. Counting bytes
// would be wrong and re-parsing the escapes would mean a second colour scanner,
// so the string is measured stripped and the spaces go on after the reset.
func padANSI(s string, n int) string {
	if pad := n - widthOf(stripANSI(s)); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

func levelByNum(n int) *level {
	for _, lv := range levels {
		if lv.num == n {
			return lv
		}
	}
	return levels[0]
}

// levelAt is the menu's cursor: levels[position-1], clamped. Positions are
// 1-based and the extra sits last, so position len(levels) is the toolbox — the
// one entry the numbers cannot reach.
func levelAt(pos int) *level {
	if pos < 1 {
		pos = 1
	}
	if pos > len(levels) {
		pos = len(levels)
	}
	return levels[pos-1]
}
