package tuxland

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/term"
)

// ANSI attributes, emitted only when our stdout is a terminal: the game adds no
// escape of its own to a pipe, so a scripted run reads as plain text. (bash's own
// sequences still come through the pty relay — bracketed paste among them — which
// is why the screen model, not a byte comparison, is what the tests read.)
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiItalic = "\x1b[3m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

// ui is the game's single voice on the terminal. Everything the game says goes
// through here, and so does the pty's output relay, so every write is
// serialized: a card never interleaves with bash's output mid-line.
//
// Every line is laid out against the terminal's real width, which is re-read on
// each use — the student's window is resized, split and maximized, and nothing
// the game draws may fall off the edge of it.
type ui struct {
	mu      sync.Mutex
	w       io.Writer
	tty     bool // our stdout is a terminal: colors on, and nobody is scripted
	color   bool
	cols    int  // 0 = ask the terminal; tests pin it
	afterCR bool // the byte we wrote last was a CR; guarded by mu
	atStart bool // the cursor sits at the start of a row, ready for text; guarded by mu
}

func newUI(w io.Writer) *ui {
	tty := false
	if f, ok := w.(*os.File); ok {
		tty = term.IsTerminal(int(f.Fd()))
	}
	// Nothing has been drawn yet, so the cursor is where a row begins.
	return &ui{w: w, tty: tty, color: tty, atStart: true}
}

// width is how many columns we have to play with, or 80 when we can't ask
// (a pipe, a test, a pty that never said).
func (u *ui) width() int {
	if u.cols > 0 {
		return u.cols
	}
	if f, ok := u.w.(*os.File); ok {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w >= 20 {
			return w
		}
	}
	return 80
}

// widthOf is how many columns s takes on screen, so a line can be measured
// before it is drawn: colours take nothing, the emoji the game uses are the one
// family of glyphs terminals draw two columns wide, and a control character
// takes no column at all — the game writes its own carriage returns, and a
// measurement that counted one would be a column wide of wrong.
func widthOf(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f: // a control character moves the cursor, it is not drawn
		case wideRune(r):
			w += 2
		default:
			w++
		}
	}
	return w
}

func wideRune(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1FAFF) || r == 0x2705 // the emoji, and ✅
}

// stripANSI drops the escape sequences: what's on screen is the glyphs.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		for i++; i < len(s) && !isANSILetter(s[i]); i++ {
		}
	}
	return b.String()
}

func isANSILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// hangFor is the indent for the lines that continue under the line `prefix`
// opens — as wide as that prefix really is, so a wrapped card lines up.
func hangFor(prefix string) string {
	return strings.Repeat(" ", widthOf(stripANSI(prefix)))
}

// rule is a horizontal rule sized to the terminal: as wide as the screen allows,
// never wider than max, and never so short that it stops looking like a rule.
// The two-space indent every line carries is already subtracted.
func (u *ui) rule(ch string, max int) string {
	n := u.width() - 2
	if n > max {
		n = max
	}
	if n < 8 {
		n = 8
	}
	return strings.Repeat(ch, n)
}

// write sends one atomic chunk of text to the terminal.
//
// On a terminal the game ends its own lines, because raw mode on stdin takes the
// kernel's newline translation away from our output too (stdin and stdout are the
// same device), and a bare LF then only moves down a row and keeps the column. Into
// a pipe there is no cursor, and the output stays plain text. The whole story, and
// the staircase it prevents: DESIGN.md §5.
func (u *ui) write(s string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.writeLocked(s)
}

// writeLocked is write with the lock already held, for the callers that need to
// look at what the last write did before deciding what to write next.
func (u *ui) writeLocked(s string) {
	if s == "" {
		return // nothing was drawn, so the cursor did not move
	}
	if u.tty {
		s = u.withCR(s)
	}
	io.WriteString(u.w, s)
	// Where the next character lands, which prompt() needs to know.
	u.atStart = strings.HasSuffix(s, "\n")
}

// withCR puts a carriage return in front of every LF that does not already have
// one. It remembers the last byte it wrote, because the relay streams bash's
// output as it reads it and a CRLF can land across two chunks.
func (u *ui) withCR(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		u.afterCR = false
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && !u.afterCR {
			b.WriteByte('\r')
		}
		u.afterCR = s[i] == '\r'
		b.WriteByte(s[i])
	}
	return b.String()
}

// line writes text as a line of its own: it opens a row first when the cursor is
// not already at the start of one.
//
// The game draws its lines wherever the student left the terminal, and the normal
// place to leave it is *parked after a `tuxland$ ` prompt*, waiting to be typed
// at — so a line written from there would land in the middle of that row. The row
// is the game's to open; the relay's bytes are not touched (see `write`), and
// neither is a menu repaint, which is the one thing that means to rewrite the row
// it is on.
//
// A blank line is already a row of its own — ending the row the cursor is in *is*
// the blank line — so it is only ever one newline.
func (u *ui) line(s string) {
	if s == "" {
		u.write("\n")
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.atStart {
		u.writeLocked("\n")
	}
	u.writeLocked(s + "\n")
}

// lines is line, under the name that says what the call site is doing. There is
// no difference in behaviour: a card is drawn in one write precisely because
// every write is atomic, so `lines` is about reading the call site, not about
// getting a block onto the screen in one piece.
func (u *ui) lines(s string) { u.line(s) }

// prose is how the game says anything longer than a line: `first` opens it (its
// colours, and exactly as wide as `hang`), and the text is wrapped to the
// terminal with a hanging indent, so a hint in a 40-column window reads like a
// paragraph instead of a wall.
func (u *ui) prose(first, hang, text string) {
	w := u.width()
	for i, ch := range wrap(text, w-widthOf(stripANSI(first)), w-widthOf(hang)) {
		if i == 0 {
			u.line(first + u.markup(ch.text))
		} else {
			u.line(hang + u.markup(ch.text))
		}
	}
}

// proseWith is prose in more than one voice: spans are laid end to end, wrapped
// together, and each line re-applies the voice of the span it came from — so a
// wrapped line never loses the "this is something you can type" colour halfway
// down, and `code` / *emphasis* spans survive the wrap.
func (u *ui) proseWith(first, hang string, spans ...span) {
	var plain strings.Builder
	for _, s := range spans {
		plain.WriteString(s.text)
	}
	body := []rune(plain.String())
	w := u.width()
	head := widthOf(stripANSI(first))

	// A label that leaves no room for the first word of its value takes a line of
	// its own, rather than squeezing the value down to a word per line.
	alone := len(body) > 0 && !unicode.IsSpace(body[0]) &&
		head+widthOf(firstWord(string(body))) > w
	if alone {
		u.line(first)
		head = 0
	}
	for i, ch := range wrap(string(body), w-head, w-widthOf(hang)) {
		pre := hang
		if i == 0 && !alone {
			pre = first
		}
		to := ch.at + len([]rune(ch.text))
		u.line(pre + u.styledAt(body, ch.at, to, spans))
	}
}

// firstWord is the token a wrapped line has to make room for first.
func firstWord(s string) string {
	if w := strings.IndexByte(s, ' '); w >= 0 {
		return s[:w]
	}
	return s
}

// span is a run of text in one voice. attr is an ANSI attribute, or "" for plain.
type span struct{ text, attr string }

// dimline is a whole quiet sentence of the game's voice, wrapped if it has to be,
// all of it in the dim voice. It carries no backticks: markup is a per-line thing
// and a one-off remark has no commands in it.
func (u *ui) dimline(text string) {
	u.proseWith("  ", "  ", span{text, ansiDim})
}

// styledAt renders body[from:to] with each span's own voice, wherever the slice
// starts: a line that continues in the middle of a dim lead stays plain, and one
// that continues inside a `code` span keeps the command's colour. A plain span
// gets the card's own markup, because its text came from a level's prose.
func (u *ui) styledAt(body []rune, from, to int, spans []span) string {
	var b strings.Builder
	at := 0
	for _, s := range spans {
		end := at + len([]rune(s.text))
		lo, hi := max(at, from), min(end, to)
		at = end
		switch {
		case lo >= hi:
		case s.attr == "":
			b.WriteString(u.markup(string(body[lo:hi])))
		default:
			b.WriteString(u.col(s.attr, string(body[lo:hi])))
		}
	}
	return b.String()
}

// chunk is one line of wrapped text: a verbatim slice of what was wrapped, with
// the rune offset it starts at, so a caller can style it by span without
// re-finding the words.
type chunk struct {
	text string
	at   int
}

// wrap breaks a card's text into lines: the first at most `first` columns (the
// line it opens has a prefix of its own), the rest at most `rest`. It breaks
// only on spaces and only where the break is due, so the words a student reads
// stay in the order they were written. A `command` is never split: if the break
// falls inside one, the whole command moves down to the next line instead, and
// a command too long for any line is left long, because half a command teaches
// the wrong lesson.
func wrap(s string, first, rest int) []chunk {
	if first < 20 {
		first = 20
	}
	if rest < 20 {
		rest = 20
	}
	var out []chunk
	for _, para := range strings.Split(s, "\n") {
		runes := []rune(para)
		code := codeRanges(runes)
		lineAt, room, end := 0, first, 0
		for i := 0; i < len(runes); {
			for i < len(runes) && runes[i] == ' ' {
				i++
			}
			if i >= len(runes) {
				break
			}
			start := i
			for i < len(runes) && runes[i] != ' ' {
				i++
			}
			end = i
			if start <= lineAt {
				continue
			}
			// The line ends just before the word that did not fit — unless that
			// would cut a `command` in half, or strand a flag on the next line.
			to, fits := start, widthOf(string(runes[lineAt:end])) <= room
			// A `command` moves down whole — but only while it could still fit a
			// line by itself. One longer than the line is split as any long word
			// is: the terminal would break it there anyway.
			if open, close := codeSpanAt(code, lineAt, start); open >= lineAt &&
				widthOf(string(runes[open:close+1])) <= room &&
				widthOf(string(runes[lineAt:close+1])) > room {
				to, fits = open, false
			}
			// A flag travels with its command: "tuxland -level 3" is one thing to
			// run, and "-level 3" alone on a line is not.
			if !fits && runes[start] == '-' {
				if before := wordBefore(runes, start, lineAt); before > lineAt {
					to = min(to, before)
				}
			}
			if fits {
				continue // the word fits on the line as it stands
			}
			for to > lineAt && runes[to-1] == ' ' {
				to--
			}
			if to <= lineAt {
				continue // the line holds no words yet: this one starts it
			}
			out = append(out, chunk{string(runes[lineAt:to]), lineAt})
			for to < len(runes) && runes[to] == ' ' {
				to++ // the next line starts at its first word, not at the gap
			}
			lineAt, room, i = to, rest, to
		}
		if lineAt < end {
			out = append(out, chunk{string(runes[lineAt:end]), lineAt})
		}
	}
	return out
}

// wordBefore is where the word in front of `at` starts, or lineAt when the word
// in front of it is not on this line.
func wordBefore(runes []rune, at, lineAt int) int {
	p := at
	for p > lineAt && runes[p-1] == ' ' {
		p--
	}
	for p > lineAt && runes[p-1] != ' ' {
		p--
	}
	return p
}

// codeRanges are the rune ranges a wrap must not break: the backticked spans.
func codeRanges(runes []rune) [][2]int {
	var out [][2]int
	open, in := 0, false
	for i, r := range runes {
		if r != '`' {
			continue
		}
		if in {
			out, in = append(out, [2]int{open, i}), false
			continue
		}
		open, in = i, true
	}
	return out
}

// codeSpanAt is the `command` the word at `at` belongs to, when that command
// begins on the line starting at lineAt — the one case where a break has to be
// moved further left, to before the command's backtick.
func codeSpanAt(ranges [][2]int, lineAt, at int) (open, close int) {
	for _, r := range ranges {
		if r[0] >= lineAt && at >= r[0] && at <= r[1] {
			return r[0], r[1]
		}
	}
	return -1, -1
}

// col wraps s in an ANSI attribute — or returns it unchanged when color is off.
func (u *ui) col(attr, s string) string {
	if !u.color || attr == "" || s == "" {
		return s
	}
	return attr + s + ansiReset
}

// code marks a real command or file name: the game's "this is something you can
// actually type" voice.
func (u *ui) code(s string) string { return u.col(ansiBold+ansiCyan, s) }

// markup renders the two inline voices of a card: `backticked` spans are real
// commands and file names, *starred* spans are emphasis.
//
// A star inside backticks is a star, not a voice: a glob is part of the command
// (`find . -name "*.log"`), and the emphasis rule used to eat it, so the card
// told the student to run `-name ".log"`. Commands are verbatim — backticks win.
func (u *ui) markup(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i, part := range strings.Split(s, "`") {
		if i%2 == 1 {
			b.WriteString(u.code(part))
			continue
		}
		for j, span := range strings.Split(part, "*") {
			if j%2 == 1 {
				span = u.col(ansiItalic, span)
			}
			b.WriteString(span)
		}
	}
	return b.String()
}

// prompt hands the terminal back to the student. The shell's own PS1 is an
// invisible marker the relay strips (see session.go), so this line is the only
// prompt on screen and a card and the shell's output can never fight over it.
//
// It always opens its own row, because the cursor is not always at the start of
// one: `clear` and Ctrl-L leave it wherever the clear put it — the top left
// corner, with nothing on the line — and a prompt written there would land
// *beside* the last thing on screen (`tuxland$ tuxland$`). So the newline is the
// game's, not bash's, and a prompt can never be drawn on top of a warning.
func (u *ui) prompt() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.atStart {
		u.writeLocked("\n")
	}
	u.writeLocked(u.col(ansiGreen+ansiBold, "tuxland$ "))
}

// table lays out the level list. Three columns when the terminal has room for
// them; otherwise each level gets two lines, so a narrow window never runs one
// level's commands into the next one's name.
//
// mark is a two-column gutter down the left: room for a ✅ on a level the student
// has finished, so the list doubles as their record without a word about it.
func (u *ui) table(rows []levelRow) {
	const indent, markW, numW, nameW, tagW = 2, 3, 3, 16, 30
	nameCol := indent + markW + numW // where a level's name starts
	w := u.width()
	cmdsW := 0
	for _, r := range rows {
		if n := widthOf(r.cmds); n > cmdsW {
			cmdsW = n
		}
	}
	wide := w >= indent+markW+numW+nameW+tagW+cmdsW

	var b strings.Builder
	for _, r := range rows {
		num := fmt.Sprint(r.num) + "  "
		if r.extra {
			num = "✦  " // the toolbox has no number: ✦ keeps the gutter straight
		}
		// The mark is a display-width gutter, not a byte one: ✅ is three bytes and
		// two columns, and the numbers have to line up whether it is there or not.
		mark := ""
		if r.mark != "" {
			mark = u.col(ansiGreen, r.mark)
		}
		if n := markW - widthOf(stripANSI(mark)); n > 0 {
			mark += strings.Repeat(" ", n)
		}
		if wide {
			fmt.Fprintf(&b, "  %s%s%s%s%s\n", mark, u.col(ansiBold, num), u.col(ansiBold, pad(r.name, nameW)),
				pad(r.tagline, tagW), u.col(ansiDim, r.cmds))
			continue
		}
		fmt.Fprintf(&b, "  %s%s%s\n", mark, u.col(ansiBold, num), u.col(ansiBold, r.name))
		for _, ch := range wrap(r.tagline+" · "+r.cmds, w-nameCol, w-nameCol) {
			fmt.Fprintf(&b, "%s%s\n", strings.Repeat(" ", nameCol), u.col(ansiDim, ch.text))
		}
	}
	u.write(b.String())
}

// pad right-pads a table cell (the level names and taglines are all ASCII, so
// counting bytes lines up).
func pad(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

// levelRow is one line of the level table, kept as plain text so the layout can
// measure it before anything is styled. mark is the progress gutter: a ✅ on a
// level the student has finished, empty otherwise. extra is the habits toolbox,
// which has no number and shows a ✦ in the number column instead.
type levelRow struct {
	num           int
	extra         bool
	name, tagline string
	cmds          string
	mark          string
}

// levelRows is the table, with this student's progress folded in — the same list
// whether the game is about to be played or is only being listed.
func levelRows(p progress) []levelRow {
	rows := make([]levelRow, 0, len(levels))
	for _, lv := range levels {
		mark := ""
		switch {
		case lv.isExtra():
			if p.isExtraDone() {
				mark = "✅ "
			}
		case p.isDone(lv.num):
			mark = "✅ " // the trailing space is the gutter's, not a typo
		}
		rows = append(rows, levelRow{
			num:     lv.num,
			extra:   lv.extra,
			name:    lv.name,
			tagline: lv.tagline,
			cmds:    strings.Join(lv.cmds, " "),
			mark:    mark,
		})
	}
	return rows
}

// bullet is one `• thing — detail` line, in two voices, wrapped under itself.
func (u *ui) bullet(item, detail string) {
	first := "   " + u.col(ansiGreen, "•") + " "
	u.proseWith(first, hangFor(first),
		span{item, ansiBold + ansiCyan}, span{"  " + detail, ansiDim})
}

// masthead is the game saying its own name, with the one-liner under it.
func (u *ui) masthead() {
	first := "  " + u.col(ansiBold+ansiCyan, "🐧 TUXLAND")
	u.lines("")
	if u.width() >= widthOf(first)+widthOf("  the guided command-line game") {
		u.line(first + "  " + u.col(ansiDim, "the guided command-line game"))
		return
	}
	u.line(first)
	u.dimline("the guided command-line game")
}

// list is `--list`: the whole game on one screen, nothing to play.
func (u *ui) list(p progress) {
	u.masthead()
	u.lines("")
	u.table(levelRows(p))
	u.lines("")
	u.line("  " + u.col(ansiBold, "play"))
	for _, b := range [][2]string{
		{"tuxland", "the menu — pick a level and go"},
		{"tuxland -level 3", "jump straight into one"},
		{"tuxland -list", "this list again"},
	} {
		u.bullet(b[0], b[1])
	}
	u.lines("")
	u.proseWith("  ", "  ", span{"your playground: ", ansiDim}, span{"~/linux-lab", ansiBold + ansiCyan},
		span{" — re-run any level any time, nothing outside is touched.", ansiDim})
	u.proseWith("  ", "  ", span{progressLine(p), ansiBold + ansiDim})
	u.lines("")
}

// progressLine is the one sentence about what the student has done, said the same
// way whatever the count: two of six is a place to be, and six of six is an
// invitation rather than a full stop. It counts the numbered levels only — the
// habits toolbox is extra, and the menu says so on its own line.
func progressLine(p progress) string {
	all := fmt.Sprintf("%d of %d levels done", p.count(), NumLevels())
	if p.count() == 0 {
		return "no levels finished yet — start anywhere"
	}
	if p.count() == NumLevels() {
		return all + " · every level open to replay"
	}
	lv := levelByNum(p.next())
	return fmt.Sprintf("%s · next up: level %d · %s", all, lv.num, lv.name)
}

// spelled counts out loud, for the one line that has to say how many levels
// there are in words: the finale's banner. Counting the menu's "1-6 to jump" is
// fmt's job; this is for the one that reads as a sentence. Past twelve it falls
// back to digits rather than inventing a word — which cannot happen today, and
// is exactly why the fallback should be dull.
func spelled(n int) string {
	words := [...]string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve"}
	if n >= 0 && n < len(words) {
		return words[n]
	}
	return strconv.Itoa(n)
}

// kickoff is the default run's opening screen: what the game is, the levels with
// this student's ✅ beside the finished ones, and the question that picks one.
// The prompt line below it belongs to the menu loop (askLevel), which repaints it
// as they move; everything above it is final.
func (u *ui) kickoff(p progress) {
	u.masthead()
	u.lines("")
	u.line("  " + u.col(ansiBold, "How it works"))
	for _, b := range []string{
		"you type REAL bash — this is a real shell on real files",
		"I watch your workspace; when a step is done I say so",
		"stuck? hints appear after a couple of tries",
		"Ctrl-D or exit leaves · your finished levels are remembered",
	} {
		first := "  " + u.col(ansiGreen, "•") + " "
		u.prose(first, hangFor(first), b)
	}
	u.lines("")
	u.line("  " + u.col(ansiBold, "Levels"))
	u.table(levelRows(p))
	u.lines("")
	u.proseWith("  ", "  ", span{progressLine(p), ansiBold + ansiYellow})
	u.lines("")
	u.proseWith("  ", "  ",
		span{"✦ the habits toolbox", ansiBold + ansiCyan},
		span{" — no number, no flag: man, Tab, ↑ and the keys that get you out.", ansiDim})
	u.lines("")
	u.proseWith("  ", "  ",
		span{"↑/↓ to move", ansiBold + ansiCyan}, span{" · ", ansiDim},
		span{fmt.Sprintf("1-%d to jump", NumLevels()), ansiBold + ansiCyan}, span{" · ", ansiDim},
		span{"Enter to start", ansiBold + ansiCyan}, span{" — and from there on it is you and bash.", ansiDim})
}

// finale is the exit ramp: tuxland is an on-ramp, the real wargames are next.
func (u *ui) finale() {
	u.lines("")
	u.line("  " + u.col(ansiBold+ansiYellow, "🎓 ALL "+spelled(NumLevels())+" LEVELS DONE"))
	u.prose("  ", "  ", "real commands, real files. now the real thing — same idea, harder:")
	// The links are padded to one column so the two URLs line up.
	const banditName = "OverTheWire Bandit"
	for _, b := range [][2]string{
		{banditName, "https://overthewire.org/wargames/bandit/"},
		{"pwn.college", "https://pwn.college/"},
	} {
		u.bullet(pad(b[0], widthOf(banditName)), b[1])
	}
	u.prose("  ", "  ", "start at Bandit Level 0 — you're already ahead of it. 🐧")
	u.lines("")
	u.proseWith("  ", "  ",
		span{"✦ still hungry? ", ansiBold + ansiCyan},
		span{"the habits toolbox is on the menu — man, Tab, ↑, Ctrl-C, Ctrl-D.", ansiDim})
	u.lines("")
}

// bye is Ctrl-D (or `exit`) mid-run: no scolding, and the playground stays put.
func (u *ui) bye(dir string, finished bool) {
	u.lines("")
	if finished {
		first := "  " + u.col(ansiBold+ansiYellow, "🎉") + " "
		u.proseWith(first, hangFor(first),
			span{"that's level ", ""}, span{"all done", ansiBold}, span{" for now.", ansiDim})
	}
	u.proseWith("  ", "  ",
		span{"🐧 leaving the playground", ansiBold},
		span{" — it stays exactly where it is: ", ansiDim}, span{tilde(dir), ansiBold + ansiCyan})
	u.proseWith("  ", "  ",
		span{"play again: ", ansiDim}, span{"tuxland", ansiBold + ansiCyan},
		span{" · one level: ", ansiDim}, span{"tuxland -level 3", ansiBold + ansiCyan},
		span{" · the list: ", ansiDim}, span{"tuxland -list", ansiBold + ansiCyan})
	u.lines("")
}
