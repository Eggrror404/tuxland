package tuxland

import (
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// keyboard is the game's one reader of the player's terminal, and it lives for
// the whole run: one raw-mode switch, one restore, one goroutine parked on stdin.
//
// It has to be shared. A reader per level would race the level before it for the
// same keystrokes, and whichever goroutine the kernel woke first would win — so
// the first line of a level could be written to the shell that had just died, and
// simply vanish. Keystrokes that arrive while no shell is listening are held, and
// dropped when the next level opens: a line typed between two levels must not skip
// the first step of the one that follows. (DESIGN.md §3 has the reasoning.)
type keyboard struct {
	mu   sync.Mutex
	cur  *session // the shell that is listening right now
	held []byte   // typed while there was nowhere to put it
	tty  bool     // a person is here; a piped script is not, and keeps its lines

	cap func([]byte) // when set, keystrokes go here instead of the shell (the exit-key beat)

	eofAt time.Time // when stdin ran out: a piped script, or an ssh session that died

	raw *term.State // the terminal as we found it, restored on close
}

// newKeyboard puts the terminal in raw mode and starts reading it. Everything the
// student types from here on is forwarded byte for byte — tab-completion, arrow
// keys, Ctrl-C, colours — so the shell behaves exactly as it would on its own.
func newKeyboard() *keyboard {
	in := os.Stdin
	k := &keyboard{tty: term.IsTerminal(int(in.Fd()))}
	if k.tty {
		if state, err := term.MakeRaw(int(in.Fd())); err == nil {
			setRaw(state)
			k.raw = state
		}
	}
	go k.read(in)
	return k
}

func (k *keyboard) read(in *os.File) {
	buf := make([]byte, 512)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			k.deliver(buf[:n])
		}
		if err != nil {
			k.markEOF()
			return
		}
	}
}

// deliver hands a chunk of keystrokes to the listening shell, or holds it until
// there is one. Enter events are made at write time, so a held line can't advance
// a step it was never delivered to.
func (k *keyboard) deliver(b []byte) {
	k.mu.Lock()
	cap := k.cap
	if cap != nil {
		k.mu.Unlock()
		cap(b) // the exit-key beat reads it, and nothing reaches bash
		return
	}
	cur := k.cur
	if cur == nil {
		k.held = append(k.held, b...) // copied: the read buffer is reused
		k.mu.Unlock()
		return
	}
	k.mu.Unlock()
	cur.keys(b)
}

// setCapture points keystrokes at f instead of the shell — the exit-key beat's
// way of reading Ctrl-D for itself. nil hands the keyboard back.
func (k *keyboard) setCapture(f func([]byte)) {
	k.mu.Lock()
	k.cap = f
	k.mu.Unlock()
}

// attach gives a shell the keyboard. Anything held is thrown away on a real
// terminal (a stray Enter must not skip step one) and delivered in full on a piped
// run, where the whole script arrived before the first shell existed.
func (k *keyboard) attach(s *session) {
	k.mu.Lock()
	k.cur = s
	held := k.held
	k.held = nil
	if k.tty {
		held = nil
	}
	eof := !k.eofAt.IsZero()
	k.mu.Unlock()

	if len(held) > 0 {
		s.keys(held)
	}
	if eof {
		// stdin died between two levels; the step that follows has to hear about it
		s.send(evEOF)
	}
}

// detach takes the keyboard back while a level ends.
func (k *keyboard) detach() {
	k.mu.Lock()
	k.cur = nil
	k.mu.Unlock()
}

// markEOF remembers that nothing more will ever be typed — a piped script running
// out, or an ssh session that dropped.
func (k *keyboard) markEOF() {
	k.mu.Lock()
	k.eofAt = time.Now()
	cur := k.cur
	k.mu.Unlock()
	if cur != nil {
		cur.send(evEOF)
	}
}

// close hands the terminal back the way it was found.
func (k *keyboard) close() {
	if k.raw != nil {
		restoreRaw(k.raw)
	}
}
