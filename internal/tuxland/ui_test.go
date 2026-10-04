package tuxland

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// The student's terminal is never the size the design was drawn at: it gets
// split, resized and maximised mid-level. So every line the game draws is laid
// out against the real width, and these tests are the lock on that — a line may
// only run off the edge when it holds a single unbreakable token (a URL, a very
// long path), because a terminal wraps those anyway.

// screen is one thing the game can print, as a caller would print it.
type screen struct {
	name string
	draw func(*game)
}

// everyScreen is every screen the game can put on the terminal, with all five
// levels' and the ✦ toolbox's real card text in it — not a fixture, the content
// itself.
//
// The kickoff and the list come in all three progress states, because the ✅
// gutter and the progress sentence are new content of different lengths, and a
// layout is only as wide as its widest honest case.
func everyScreen() []screen {
	screens := []screen{
		{"list", func(g *game) { g.ui.list(newProgress()) }},
		{"list some done", func(g *game) { g.ui.list(progressOf(1, 2, 3)) }},
		{"kickoff", func(g *game) { g.ui.kickoff(newProgress()) }},
		{"kickoff some done", func(g *game) { g.ui.kickoff(progressOf(1, 2, 3)) }},
		{"kickoff all done", func(g *game) { g.ui.kickoff(progressOf(1, 2, 3, 4, 5, 6)) }},
		{"finale", func(g *game) { g.ui.finale() }},
		{"bye", func(g *game) { g.ui.bye(g.dir, false) }},
		{"bye finished", func(g *game) { g.ui.bye(g.dir, true) }},
	}
	// The menu's own line, at every level: the name is the widest part and the
	// narrow-window fallback drops it, so both have to survive the width tests.
	for n := 1; n <= len(levels); n++ {
		n := n
		screens = append(screens, screen{
			"menu prompt " + fmt.Sprint(n),
			func(g *game) { g.ui.menuPrompt(n, 0) },
		})
	}
	for _, lv := range levels {
		lv := lv
		screens = append(screens,
			screen{"banner " + lv.name, func(g *game) { g.banner(lv) }},
			screen{"level complete " + lv.name, func(g *game) { g.levelComplete(lv) }},
		)
		for _, sec := range lv.sections {
			sec := sec
			screens = append(screens, screen{
				"section " + lv.name + "/" + sec.title,
				func(g *game) { g.sectionHeader(sec, 1) },
			})
			for _, s := range sec.steps {
				s := s
				hint := (&stepState{step: &s}).hint(1)
				name := lv.name + "/" + sec.title + "/" + firstWords(s.goal, 3)
				screens = append(screens,
					screen{"card " + name, func(g *game) { g.card("▶", s.goal, s.note) }},
					screen{"ok " + name, func(g *game) { g.ok(s.done) }},
					screen{"cheer " + name, func(g *game) { g.cheer(&s) }},
					screen{"warn " + name, func(g *game) { g.warn("not there yet", hint) }},
				)
			}
		}
	}
	return screens
}

// render draws one screen into a buffer at a pinned width, as a pipe.
func render(t *testing.T, cols int, color bool, s screen) string {
	t.Helper()
	return renderOn(t, cols, false, color, s)
}

// renderOn draws one screen, choosing what the writer is: a terminal (colors on,
// and the game writes its own carriage returns) or a pipe (plain text).
func renderOn(t *testing.T, cols int, tty, color bool, s screen) string {
	t.Helper()
	var buf bytes.Buffer
	u := newUI(&buf)
	u.tty = tty
	u.color = tty && color
	u.cols = cols
	g := &game{ui: u, dir: "/home/freshman/linux-lab/level-01-navigation"}
	s.draw(g)
	return buf.String()
}

// firstWords is enough of a goal to tell two steps apart in a test failure.
func firstWords(s string, n int) string {
	words := strings.Fields(s)
	if len(words) > n {
		words = words[:n]
	}
	return strings.Join(words, " ")
}

// TestNothingOverrunsTheTerminal is the one the design asks for: whatever the
// window is, every line the game draws fits inside it. The one thing that may
// run over is a line that ends in a single unbreakable token — a URL, a very
// long path — which no amount of wrapping can help.
func TestNothingOverrunsTheTerminal(t *testing.T) {
	for _, cols := range []int{40, 48, 56, 64, 72, 80, 100, 120, 200} {
		for _, color := range []bool{false, true} {
			for _, s := range everyScreen() {
				for i, line := range strings.Split(render(t, cols, color, s), "\n") {
					text := strings.TrimRight(stripANSI(line), " ")
					if w := widthOf(text); w > cols && !unbreakable(text, cols) {
						t.Errorf("%s at %d cols (color=%v): line %d is %d wide: %q",
							s.name, cols, color, i+1, w, text)
					}
				}
			}
		}
	}
}

// unbreakable reports whether a line runs over only because the token it ends in
// could not be broken up: a path, a URL, anything with no space in it.
func unbreakable(line string, cols int) bool {
	words := strings.Fields(line)
	if len(words) == 0 {
		return true
	}
	return widthOf(words[len(words)-1])+2 > cols
}

// TestNoEscapeIsEverCut guards the other half of the layout: colours are applied
// to whole spans, never spliced into the middle of an escape sequence (which
// would make the rest of the screen render in the wrong voice).
func TestNoEscapeIsEverCut(t *testing.T) {
	for _, cols := range []int{40, 80, 120} {
		for _, s := range everyScreen() {
			out := render(t, cols, true, s)
			if got, want := out, stripANSI(out); stripANSI(got) != want {
				t.Fatalf("%s at %d cols: escapes left in the text: %q", s.name, cols, got)
			}
		}
	}
}

// TestTheStudentSeesNoStaircase feeds every screen through the smallest terminal
// model that can show the bug: a CR returns to column 0, an LF moves down one row
// and *keeps* the column, and a line that runs off the right edge wraps.
//
// The game makes the student's terminal raw so keystrokes reach bash verbatim,
// and on a terminal that is stdin and stdout at once — so the kernel stops
// translating the game's own newlines, and a program that leaves the carriage
// return to the kernel slides every line right by the length of the one above it.
// The bytes are correct in that case, so only a model of the screen can see it.
func TestTheStudentSeesNoStaircase(t *testing.T) {
	for _, cols := range []int{40, 80, 120} {
		for _, s := range everyScreen() {
			m := newScreenModel(cols)
			m.feed(renderOn(t, cols, true, true, s))
			for i, col := range m.starts {
				if col != 0 {
					t.Errorf("%s at %d cols: line %d begins at column %d, not 0 — the terminal slid it right",
						s.name, cols, i+2, col)
				}
			}
		}
	}
}

// screenModel is the part of a terminal that decides where a line lands: CR
// returns to column 0, LF indexes down a row without moving, and the right edge
// wraps.
//
// It keeps the grid as well as the column each line of output began at, because
// a screen has two things to be wrong about: where a line starts (a staircase
// down the right, which the bytes never show) and what it says (which is what
// looking at the grid is for).
type screenModel struct {
	cols   int
	row    int
	col    int
	grid   [][]string // one cell per column; "" where a wide glyph continues
	starts []int      // the column each line of output began at
}

func newScreenModel(cols int) *screenModel {
	return &screenModel{cols: cols, grid: [][]string{make([]string, cols)}}
}

// newline is a line of output finishing: the next one starts wherever the
// cursor now is, which is column 0 when the program ended its line with a
// carriage return and the length of the line above it when it did not.
func (m *screenModel) newline() {
	m.starts = append(m.starts, m.col)
	m.rowDown()
}

// wrap is the right edge: the line continues on the next row, at the margin.
func (m *screenModel) wrap() {
	m.starts = append(m.starts, 0)
	m.rowDown()
	m.col = 0
}

func (m *screenModel) rowDown() {
	m.row++
	m.grid = append(m.grid, make([]string, m.cols))
}

func (m *screenModel) put(ch string, w int) {
	if m.col >= m.cols {
		m.wrap()
	}
	m.grid[m.row][m.col] = ch
	for c := 1; c < w && m.col+c < m.cols; c++ {
		m.grid[m.row][m.col+c] = ""
	}
	m.col += w
}

func (m *screenModel) feed(s string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\r':
			m.col = 0
		case '\n':
			m.newline() // down a row, the column is kept
		case '\b':
			m.col = max(0, m.col-1)
		default:
			// One escape sequence is not a glyph; the layout never has to
			// measure through one, so it is stepped over whole.
			if s[i] == 0x1b {
				for i++; i < len(s) && !isANSILetter(s[i]); i++ {
				}
				continue
			}
			r, size := utf8.DecodeRuneInString(s[i:])
			w := 1 // how many columns it takes, which is not how many bytes
			if wideRune(r) {
				w = 2
			}
			m.put(string(r), w)
			i += size - 1
		}
	}
}

// rows is the text on each line of the screen, trailing blanks trimmed.
func (m *screenModel) rows() []string {
	out := make([]string, 0, len(m.grid))
	for _, r := range m.grid {
		out = append(out, strings.TrimRight(strings.Join(r, ""), " "))
	}
	return out
}

// text is every row joined by a single space. A line the game wrapped is
// rejoined, because wrapping drops the space the break consumed.
func (m *screenModel) text() string {
	return strings.Join(m.rows(), " ")
}

// TestAScriptedRunStaysClean: into a pipe there is no cursor, so the game writes
// plain newlines and the smoke test's output is plain text — the same rule the
// colours follow.
func TestAScriptedRunStaysClean(t *testing.T) {
	for _, s := range everyScreen() {
		if out := renderOn(t, 80, false, true, s); strings.Contains(out, "\r") {
			t.Errorf("%s: a carriage return reached a pipe: %q", s.name, out)
		}
	}
}

// TestAStandardTerminalKeepsTheLookTheDesignDraws locks the 80-column look: at
// the width a lecture room usually has, the decorations are the sizes §5
// specifies, and a card is the short block a card is supposed to be.
func TestAStandardTerminalKeepsTheLookTheDesignDraws(t *testing.T) {
	u := newUI(&bytes.Buffer{})
	u.cols = 80
	if got, want := widthOf(u.rule("═", 44)), 44; got != want {
		t.Errorf("banner bar is %d wide at 80 cols, want %d", got, want)
	}
	if got, want := widthOf(u.rule("─", 48)), 48; got != want {
		t.Errorf("section rule is %d wide at 80 cols, want %d", got, want)
	}
	// A wide window does not get wider decorations: the design's proportions.
	u.cols = 200
	if got, want := widthOf(u.rule("═", 44)), 44; got != want {
		t.Errorf("banner bar is %d wide at 200 cols, want %d", got, want)
	}

	for _, lv := range levels {
		for _, sec := range lv.sections {
			for _, s := range sec.steps {
				var buf bytes.Buffer
				u := newUI(&buf)
				u.cols = 80
				g := &game{ui: u, dir: "/home/freshman/linux-lab/level-01-navigation"}
				g.card("▶", s.goal, s.note)
				// Whole lines only: the prompt is written without a newline of
				// its own, and is not part of the card.
				if n := strings.Count(strings.TrimRight(buf.String(), "\n"), "\n"); n > 8 {
					t.Errorf("%s/%s: card is %d lines at 80 cols, want at most 8",
						lv.name, firstWords(s.goal, 3), n)
				}
			}
		}
	}
}

// TestWrapIsGreedyAndVerbatim covers the wrap itself: it fills each line as far
// as it can, breaks only where it has to, and hands back untouched slices of the
// text (so a caller can style by offset).
func TestWrapIsGreedyAndVerbatim(t *testing.T) {
	const text = "one two three four five six seven eight nine ten eleven twelve"
	var got []string
	for _, c := range wrap(text, 20, 20) {
		got = append(got, c.text)
		if widthOf(c.text) > 20 {
			t.Errorf("line %q is %d wide, want <= 20", c.text, widthOf(c.text))
		}
		if c.text != string([]rune(text)[c.at:c.at+len([]rune(c.text))]) {
			t.Errorf("chunk %q is not the slice it claims (%d)", c.text, c.at)
		}
	}
	want := []string{
		"one two three four", "five six seven eight", "nine ten eleven", "twelve",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("wrapped to:\n  %q\nwant:\n  %q", got, want)
	}

	// The opening line is narrower than the rest when a prefix hangs off it, and
	// it keeps the space that introduces the text.
	got = got[:0]
	for _, c := range wrap("1234567890 1234567890 1234567890 1234567890", 20, 40) {
		got = append(got, c.text)
	}
	if strings.Join(got, "|") != "1234567890|1234567890 1234567890 1234567890" {
		t.Errorf("a 20/40 wrap gave %q", got)
	}
	if c := wrap(" a b", 40, 40); len(c) != 1 || c[0].text != " a b" {
		t.Errorf("the opening space was dropped: %q", c)
	}

	// Inner spacing survives (the bye line's "  ·  " separators are load-bearing).
	if c := wrap("a  ·  b", 20, 20); len(c) != 1 || c[0].text != "a  ·  b" {
		t.Errorf("double spaces were not preserved: %q", c)
	}
	// A word longer than the width is left whole: half a path is worse than one
	// long line.
	long := "/home/freshman/linux-lab/level-01-navigation"
	if c := wrap("your playground: "+long, 24, 24); len(c) != 2 ||
		c[0].text != "your playground:" || c[1].text != long {
		t.Errorf("long path was split: %q", c)
	}
	// The emoji count as the two columns a terminal gives them: this line is 21
	// wide, so it only breaks where it does because of the emoji.
	got = got[:0]
	for _, c := range wrap("aaaaaaaaa 🐧 bbbbbbbb", 20, 20) {
		got = append(got, c.text)
	}
	if len(got) != 2 || widthOf(got[0]) > 20 {
		t.Errorf("emoji were not measured as two columns: %q", got)
	}
	// A `command` moves down whole rather than being cut in half.
	got = got[:0]
	for _, c := range wrap("read the log with `grep -c ERROR access.log` now", 28, 28) {
		got = append(got, c.text)
	}
	if strings.Join(got, "|") != "read the log with|`grep -c ERROR access.log`|now" {
		t.Errorf("a code span was split: %q", got)
	}
	// …unless it is longer than a whole line, which no layout can help: then it
	// breaks like any other over-long word.
	got = got[:0]
	for _, c := range wrap("a `echo 'cat message.txt' >> tools/hello` b", 24, 24) {
		got = append(got, c.text)
		if widthOf(c.text) > 24 {
			t.Errorf("line %q is %d wide, want <= 24", c.text, widthOf(c.text))
		}
	}
	if strings.Join(got, " ") != "a `echo 'cat message.txt' >> tools/hello` b" {
		t.Errorf("the command lost text: %q", got)
	}
}

// TestCodeSpansStayWholeAndStyled: backticks in a level's prose are the game's
// "this is something you can type" voice, and they stay that way across a wrap.
func TestCodeSpansStayWholeAndStyled(t *testing.T) {
	var buf bytes.Buffer
	u := newUI(&buf)
	u.cols = 46
	u.color = true
	g := &game{ui: u, dir: "/home/freshman/linux-lab/level-01-navigation"}
	g.warn("not there yet", "run `grep -c ERROR access.log` and read the number")
	out := buf.String()
	if strings.Contains(out, "`") {
		t.Errorf("backticks reached the screen: %q", stripANSI(out))
	}
	if !strings.Contains(out, ansiBold+ansiCyan+"grep -c ERROR access.log"+ansiReset) {
		t.Errorf("the command lost its voice: %q", stripANSI(out))
	}
}

// TestAWrappedHintKeepsItsVoice: the voice belongs to the span, not to where the
// line falls. A hint's dim lead ends on the first line; the rest of the hint,
// wrapped or not, is plain. (A narrow window is where this shows.)
func TestAWrappedHintKeepsItsVoice(t *testing.T) {
	var buf bytes.Buffer
	u := newUI(&buf)
	u.cols = 40
	u.color = true
	g := &game{ui: u, dir: "/home/freshman/linux-lab/level-01-navigation"}
	g.warn("not there yet", "words words words words words words words words words")
	lines := strings.Split(strings.TrimRight(stripANSI(buf.String()), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("the hint did not wrap: %q", lines)
	}
	if !strings.Contains(buf.String(), ansiDim+"not there yet") {
		t.Errorf("the lead is not dim: %q", buf.String())
	}
	if strings.Contains(stripANSI(buf.String()), ansiDim+"words") {
		t.Errorf("the hint is dimmed: %q", lines[1])
	}
	for _, l := range lines[1:] {
		if strings.Contains(buf.String(), ansiDim+strings.TrimSpace(stripANSI(l))) {
			t.Errorf("a continuation line was painted dim: %q", l)
		}
	}
}

// TestHangAlignsUnderItsPrefix keeps a wrapped line under the line it continues.
func TestHangAlignsUnderItsPrefix(t *testing.T) {
	for _, first := range []string{
		"  " + "▶" + " ",  // card
		"  " + "✅" + " ",  // ok / level complete
		"   " + "•" + " ", // bullet
		"  " + "🌱" + " ",  // banner
		"      ",          // a card note
	} {
		if got, want := widthOf(hangFor(first)), widthOf(first); got != want {
			t.Errorf("hangFor(%q) is %d wide, want %d", first, got, want)
		}
	}
}

// TestMarkupKeepsAGlobIntact: a star inside backticks is part of the command, so
// the emphasis rule must not eat it. This is how a card said `-name ".log"`.
func TestMarkupKeepsAGlobIntact(t *testing.T) {
	u := &ui{cols: 80} // colourless: markup's markers come off, the text stays
	for _, c := range []struct{ in, want string }{
		{"`find . -name \"*.log\"`", `find . -name "*.log"`},
		{"`cat notes/*.txt`", `cat notes/*.txt`},
		{"`grep 404 access.log | tee out.txt`", `grep 404 access.log | tee out.txt`},
		{"the quotes stop the shell from expanding `*` itself", "the quotes stop the shell from expanding * itself"},
		// Prose emphasis is the other half of the rule, and it still works.
		{"the folder *and* everything inside it", "the folder and everything inside it"},
		{"`mv` moves *and* renames", "mv moves and renames"},
	} {
		if got := stripANSI(u.markup(c.in)); got != c.want {
			t.Errorf("markup(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
