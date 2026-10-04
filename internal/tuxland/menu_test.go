package tuxland

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// The menu is the one screen where the game reads the keyboard itself, so these
// are its whole contract: which keys do what, and — the part that is easy to get
// wrong — what happens when a terminal hands an escape sequence over in pieces.

// TestMenuKeysMoveTheSelection is the everyday case: arrows, numbers, and Enter
// to commit. The number path matters for a student who does not know arrows move
// a menu; the arrow path matters for one who would rather not count.
func TestMenuKeysMoveTheSelection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		from  int
		keys  string
		level int
		done  bool
		quit  bool
	}{
		{"down once", 1, "\x1b[B", 2, false, false},
		{"up once", 3, "\x1b[A", 2, false, false},
		{"down three", 1, "\x1b[B\x1b[B\x1b[B", 4, false, false},
		{"the arrow form terminals in cursor mode send", 1, "\x1bOB", 2, false, false},
		{"a number jumps", 1, "4", 4, false, false},
		{"a number after an arrow wins", 1, "\x1b[B\x1b[B5", 5, false, false},
		{"the extra has no number", 5, "7", 5, false, false},
		{"enter commits", 1, "\r", 1, true, false},
		{"enter as a newline commits", 1, "\n", 1, true, false},
		{"move then commit", 1, "\x1b[B\r", 2, true, false},
		{"a number then commit", 5, "2\r", 2, true, false},
		{"ctrl-d leaves", 1, "\x04", 1, false, true},
		{"ctrl-c leaves", 1, "\x03", 1, false, true},
		{"a level that does not exist is ignored", 1, "9", 1, false, false},
		{"words are not commands here", 1, "level two", 1, false, false},
		{"a number above the last level is ignored", len(levels), "9", len(levels), false, false},
		{"up at the top stays", 1, "\x1b[A", 1, false, false},
		{"down at the bottom stays", len(levels), "\x1b[B", len(levels), false, false},
		{"up and down cancel", 1, "\x1b[B\x1b[B\x1b[A", 2, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, leftover := menuKey(tc.from, []byte(tc.keys))
			if m.level != tc.level || m.done != tc.done || m.quit != tc.quit {
				t.Errorf("%q from %d: level=%d done=%v quit=%v; want level=%d done=%v quit=%v",
					tc.keys, tc.from, m.level, m.done, m.quit, tc.level, tc.done, tc.quit)
			}
			if len(leftover) != 0 {
				t.Errorf("%q left %q over, which nothing will ever read", tc.keys, leftover)
			}
		})
	}
}

// An escape sequence is three bytes and the terminal does not promise to hand
// them over together. Every split has to move the selection exactly once — twice
// would skip a level, and not at all would look like a broken keyboard.
func TestAnArrowKeySplitAcrossReadsStillMovesOnce(t *testing.T) {
	for _, tc := range []struct {
		name  string
		from  int
		split [][]byte // consecutive reads
		want  int
	}{
		{"after the escape", 1, [][]byte{[]byte("\x1b"), []byte("[B")}, 2},
		{"after the introducer", 1, [][]byte{[]byte("\x1b["), []byte("B")}, 2},
		{"up, split the same way", 2, [][]byte{[]byte("\x1b"), []byte("[A")}, 1},
		{"one byte at a time", 1, [][]byte{[]byte("\x1b"), []byte("["), []byte("B")}, 2},
		{"a whole arrow and half of the next", 1, [][]byte{[]byte("\x1b[B\x1b"), []byte("[B")}, 3},
		{"a parameterised sequence, split", 1, [][]byte{[]byte("\x1b[1"), []byte(";5A")}, 1},
		{"a parameterised sequence whole", 1, [][]byte{[]byte("\x1b[1;5B")}, 2},
		{"a lone escape, then a letter", 1, [][]byte{[]byte("\x1b"), []byte("z")}, 1},
		{"an escape we do not use", 1, [][]byte{[]byte("\x1b[H"), []byte("j")}, 1},
		{"a bracketed paste, then a number", 1, [][]byte{[]byte("\x1b[200~"), []byte("4")}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			choice, pending := tc.from, []byte(nil)
			for _, keys := range tc.split {
				m, leftover := menuKey(choice, append(append([]byte(nil), pending...), keys...))
				choice, pending = m.level, leftover
			}
			if choice != tc.want {
				t.Errorf("selection ended at %d, want %d", choice, tc.want)
			}
			if len(pending) != 0 {
				t.Errorf("%d bytes were left unread: %q", len(pending), pending)
			}
		})
	}
}

// The repaint is the menu's only visual trick, and it rests on the game
// measuring its own line: a shorter line has to be padded over the longer one it
// replaces, or the tail of a name stays on screen. The screen model judges it,
// because a check on the bytes cannot see what a terminal does with a carriage
// return.
func TestARepaintReplacesTheLineItself(t *testing.T) {
	for _, cols := range []int{20, 40, 80} {
		t.Run(strconv.Itoa(cols)+" columns", func(t *testing.T) {
			// Longest first, then shortest, then back to the longest: the first
			// repaint has to cover "navigation" and the last has to un-cover it.
			rows := playMenu(t, cols, 1, 6, 1)
			// One row for the kickoff's last line, one for the prompt. A repaint
			// that spilled onto a third would be a menu drawn over its own text.
			if len(rows) != 2 {
				t.Fatalf("the prompt line took %d rows, want 2: %q", len(rows), rows)
			}
			want := "tuxland ▶ 1 navigation"
			if cols < widthOf(want)+1 {
				want = "tuxland ▶ 1" // too narrow for the name, by design
			}
			if got := strings.TrimRight(rows[1], " "); got != want {
				t.Errorf("the line reads %q, want %q", got, want)
			}
		})
	}
}

// The menu's name check has to see through the colours. Only the pty ever draws
// with them (a pipe renders plain), and a measurement that counted the colour
// escapes as columns would drop the level name on a terminal that is plenty wide
// for it. This is the coloured half of the width story; the piped half is in
// ui_test.go, where there are no escapes to miscount.
func TestTheMenuNamesTheLevelInColour(t *testing.T) {
	for _, cols := range []int{40, 80} {
		t.Run(fmt.Sprintf("%d columns", cols), func(t *testing.T) {
			var buf bytes.Buffer
			u := newUI(&buf)
			u.tty, u.color, u.cols = true, true, cols
			u.menuPrompt(1, 0)
			if row := stripANSI(buf.String()); !strings.Contains(row, "navigation") {
				t.Errorf("the menu drops the level name at %d columns: %q", cols, row)
			}
		})
	}
}

// playMenu draws the prompt line and then repaints it once per choice, exactly as
// askLevel does, and hands back the screen the student would be looking at.
func playMenu(t *testing.T, cols int, choices ...int) []string {
	t.Helper()
	if len(choices) == 0 {
		t.Fatal("playMenu needs at least one choice to draw")
	}
	var buf bytes.Buffer
	u := newUI(&buf)
	u.cols = cols
	m := newScreenModel(cols)
	m.feed("\n") // the kickoff's last line ended here; the prompt starts the next
	widest := u.menuPrompt(choices[0], 0)
	m.feed(buf.String())
	for _, c := range choices[1:] {
		buf.Reset()
		widest = u.menuPrompt(c, widest)
		m.feed(buf.String())
	}
	return m.rows()
}
