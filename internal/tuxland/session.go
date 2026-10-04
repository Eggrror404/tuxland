package tuxland

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

type eventKind int

const (
	evEnter eventKind = iota // the student submitted a line (CR or LF)
	evEOF                    // our stdin is done — piped/scripted run
	evExit                   // bash is gone (Ctrl-D, `exit`, or the pty closed)
	evFlag                   // the armed flag token was printed on screen
	evIdle                   // bash printed its prompt: it has finished answering
)

type event struct{ kind eventKind }

// A session is a real bash, running inside a pseudo-terminal, with the keyboard
// wired to it byte for byte: tab-completion, arrows, Ctrl-C and colours all behave
// exactly as they would in a plain ssh session. The keyboard itself is the game's,
// shared by every level (see keyboard.go).
//
// The game can see *when* a line was submitted and *what bash printed*. It
// deliberately never looks at what was typed — that is what keeps the shell
// authentic, and it is why every task is graded on the filesystem instead.
type session struct {
	cmd    *exec.Cmd
	ptmx   *os.File
	events chan event
	wait   chan struct{} // closed when bash is gone
	ready  chan struct{} // closed when bash has printed its first prompt

	mu    sync.Mutex
	token string // the armed flag token, empty when no hunt is active
	tail  []byte // trailing bytes, so a token split across two reads still matches

	lastOut atomic.Int64 // UnixNano of the last byte bash printed
	started atomic.Bool  // has bash printed a prompt yet?

	readyOnce sync.Once
	closeOnce sync.Once
}

// bashReady is the whole of PS1. bash prints it as soon as it is interactive and
// done starting up, and the relay drops it, so it is never on screen. The game
// waits for the first one before showing the first card: keystrokes that reach a
// pty *before* bash has set its own termios are flushed by it, and a student (or a
// script) typing at the very start of a level would otherwise lose the first line.
//
// A word joiner, not a control character: readline silently swallows some of them
// (U+0001 among them), and this one is invisible even if a chunk boundary ever let
// half of it through.
const bashReady = '\u2060' // word joiner: an invisible character bash will still print

// bashReadyBytes is the same marker as the relay sees it.
var bashReadyBytes = []byte(string(bashReady))

func newSession(dir string, u *ui) (*session, error) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		return nil, fmt.Errorf("tuxland needs bash on your PATH: %w", err)
	}

	cmd := exec.Command(bash, "--norc", "--noprofile", "-i")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"PS1="+string(bashReady), // the game draws the prompt; this rune only says "I'm up"
		"PS2=> ",                 // continuation lines look sane
		"HISTFILE=/dev/null",     // nothing outside the playground is touched
		"INPUTRC=/dev/null",      // a stray ~/.inputrc must not remap the student's keys
		"TUXLAND=1",
	)

	ptmx, err := pty.StartWithSize(cmd, winsize())
	if err != nil {
		return nil, fmt.Errorf("tuxland: cannot start bash: %w", err)
	}

	s := &session{
		cmd:    cmd,
		ptmx:   ptmx,
		events: make(chan event, 64),
		wait:   make(chan struct{}),
		ready:  make(chan struct{}),
	}
	go s.relayOutput(u)
	go s.watchWinsize()
	go func() {
		_ = cmd.Wait()
		close(s.wait)
		s.send(evExit)
	}()
	// Don't hand the level over to the student until bash is actually listening.
	select {
	case <-s.ready:
	case <-s.wait: // died on the way up; the reader will report it
	case <-time.After(2 * time.Second):
	}
	return s, nil
}

// send never blocks the reader or the relay: the event queue is a nudge, not a
// mailbox. It is sized far larger than a student can type in one burst, because
// dropping an event loses a keystroke and a full queue only costs a little latency.
func (s *session) send(k eventKind) {
	select {
	case s.events <- event{kind: k}:
	default:
	}
}

// relayOutput streams bash's output to our terminal verbatim, and watches it
// for the armed flag token while a hunt is active. The prompt marker is stripped
// on the way past — the game prints its own prompt, one that can never collide
// with a card.
func (s *session) relayOutput(u *ui) {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			out := buf[:n]
			// The marker is bash saying it is done: everything it had to say
			// is in this stream, ahead of the marker, and it will not print
			// again until the next line. That is the shell's own "you may
			// speak now", so the game can answer at once instead of guessing
			// how long a command takes — and unlike a fixed wait it is never
			// wrong about a slow command.
			idle := bytes.Contains(out, bashReadyBytes)
			if idle {
				s.readyOnce.Do(func() { close(s.ready) })
				out = bytes.ReplaceAll(out, bashReadyBytes, nil)
			}
			if len(out) > 0 || idle {
				// a bare prompt counts as bash talking too, for the quiet gap
				s.lastOut.Store(time.Now().UnixNano())
			}
			if len(out) > 0 {
				u.write(string(out))
				s.scanFlag(out)
			}
			if idle && s.promptSeen() {
				// Only now, with those bytes on the screen: the game has to
				// answer *below* the output the command just produced, and
				// both go through the same writer in this order.
				s.send(evIdle)
			}
		}
		if err != nil {
			return
		}
	}
}

// keys writes a chunk of keystrokes to bash untouched — the keyboard owns stdin
// and decides which shell gets them — and reports the one thing the game needs to
// know: that a line was submitted.
func (s *session) keys(b []byte) {
	if _, err := s.ptmx.Write(b); err != nil {
		return
	}
	for _, c := range b {
		if c == '\r' || c == '\n' {
			s.send(evEnter)
			return
		}
	}
}

// promptSeen counts this prompt marker and reports whether bash had printed one
// already. The *first* prompt is bash saying it is up and nothing else: the level's
// first card answers for it by drawing the game's own prompt line underneath, so an
// event there would be a prompt with nothing to say behind it. Every prompt after
// that is one the game owes a line for — the student's command finishing, or a
// Ctrl-C they pressed on their own.
func (s *session) promptSeen() bool { return s.started.Swap(true) }

// lastOutput is when bash last printed something — the game uses it to stay
// quiet until bash has finished answering.
func (s *session) lastOutput() time.Time { return time.Unix(0, s.lastOut.Load()) }

func (s *session) arm(token string) {
	s.mu.Lock()
	s.token, s.tail = token, nil
	s.mu.Unlock()
}

func (s *session) disarm() {
	s.mu.Lock()
	s.token, s.tail = "", nil
	s.mu.Unlock()
}

// scanFlag fires once, the moment the armed token shows up on screen. The token
// has to be *rendered* by a real command (cat / grep / a script), not merely
// exist on disk — so the student has to actually find it.
func (s *session) scanFlag(chunk []byte) {
	s.mu.Lock()
	token := s.token
	if token == "" {
		s.mu.Unlock()
		return
	}
	s.tail = append(s.tail, chunk...)
	if bytes.Contains(s.tail, []byte(token)) {
		s.token, s.tail = "", nil
		s.mu.Unlock()
		s.send(evFlag)
		return
	}
	// Keep just enough of a tail that a token straddling two reads still matches.
	if keep := len(token) - 1; len(s.tail) > keep {
		s.tail = append([]byte(nil), s.tail[len(s.tail)-keep:]...)
	}
	s.mu.Unlock()
}

// close ends the shell politely: a hangup, then the master, so the level's folder
// is never held by a stray shell with a dead terminal. Closing the master alone is
// not enough — on this pty it does not hang up the slave, and bash would sit there
// waiting on a terminal that will never answer again.
func (s *session) close() {
	s.closeOnce.Do(func() {
		_ = s.cmd.Process.Signal(syscall.SIGHUP)
		_ = s.ptmx.Close()
		select {
		case <-s.wait:
		case <-time.After(250 * time.Millisecond):
			// SIGHUP can be ignored; SIGKILL cannot.
			_ = s.cmd.Process.Signal(syscall.SIGKILL)
			<-s.wait
		}
	})
}

func (s *session) watchWinsize() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)
	for range ch {
		_ = pty.Setsize(s.ptmx, winsize())
	}
}

// winsize is the size of the pty bash runs in, and it has to be the size the game
// draws to — or bash wraps its own output at a different column than the game lays
// its cards out to, and the two disagree in front of the student. The game draws to
// stdout, so stdout decides; stdin is the same terminal in every ordinary run and
// only stands in for a stdout that cannot be asked (a pipe, a test).
func winsize() *pty.Winsize {
	for _, f := range []*os.File{os.Stdout, os.Stdin} {
		if w, h, err := term.GetSize(int(f.Fd())); err == nil && w > 0 && h > 0 {
			return &pty.Winsize{Cols: uint16(w), Rows: uint16(h)}
		}
	}
	return &pty.Winsize{Cols: 80, Rows: 24}
}

// Raw mode is held for the life of the run (see keyboard.go), so the terminal is
// always handed back — including when a signal kills us.
var (
	rawMu    sync.Mutex
	rawState *term.State
)

func setRaw(state *term.State) {
	rawMu.Lock()
	rawState = state
	rawMu.Unlock()
}

func restoreRaw(state *term.State) {
	rawMu.Lock()
	if rawState == state {
		rawState = nil
	}
	rawMu.Unlock()
	_ = term.Restore(int(os.Stdin.Fd()), state)
}

// RestoreRaw is the signal handler's way out: a student who hits Ctrl-C at
// the wrong moment must not be left with a terminal that swallows their keys.
func RestoreRaw() {
	rawMu.Lock()
	state := rawState
	rawState = nil
	rawMu.Unlock()
	if state != nil {
		_ = term.Restore(int(os.Stdin.Fd()), state)
	}
}
