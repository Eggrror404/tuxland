package tuxland

import (
	"bytes"
	"io"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// session_test.go covers the two pieces of session.go that decide *when the game
// may speak*: the prompt marker, which is the shell's own "you may answer now",
// and the foreground process, which says whether bash is the one holding the
// terminal. Both are checked here at the cheap layer — byte logic, and one real
// bash — rather than left to a 50-second wait on a pty.

// TestAMarkerSplitAcrossTwoReadsIsStillHeard: a read from a pty ends wherever it
// ends, so the three-byte marker can be cut in half, and a dropped marker leaves
// that beat unanswered — no ✅, no prompt, and half an invisible character in the
// output. Every split point is checked, and so is the byte count: the marker must
// be heard exactly once and must never reach the screen.
func TestAMarkerSplitAcrossTwoReadsIsStillHeard(t *testing.T) {
	const before, after = "the quick brown fox\r\n", "jumped over\r\n"
	marker := []byte(string(bashReady))
	for cut := 0; cut <= len(marker); cut++ {
		s := &session{}
		var got bytes.Buffer
		found := 0
		for _, chunk := range [][]byte{
			[]byte(before + string(marker[:cut])),
			[]byte(string(marker[cut:]) + after),
		} {
			out, idle := s.takeMarker(chunk)
			if idle {
				found++
			}
			got.Write(out)
		}
		if found != 1 {
			t.Errorf("marker cut after %d bytes: heard %d prompts, want 1", cut, found)
		}
		if want := before + after; got.String() != want {
			t.Errorf("marker cut after %d bytes: the screen got %q, want %q", cut, got.String(), want)
		}
	}
}

// TestAChunkThatEndsInOrdinaryTextIsNeverHeldBack is the other half of the rule:
// only a real prefix of the marker is held for the next read. Holding bytes back
// "just in case" would delay ordinary output — the last characters of a man page
// would sit invisible until the pager happened to write again.
func TestAChunkThatEndsInOrdinaryTextIsNeverHeldBack(t *testing.T) {
	for _, chunk := range []string{
		"", "\n", "done", "\x1b[?2004l", "\xe2\x81", // …but a real prefix is held
	} {
		s := &session{}
		out, idle := s.takeMarker([]byte(chunk))
		if idle {
			t.Errorf("%q was taken for a prompt", chunk)
		}
		if want := chunk; chunk != "\xe2\x81" && string(out) != want {
			t.Errorf("%q was held back: the screen got %q", chunk, out)
		}
	}
	if out, _ := (&session{}).takeMarker([]byte("\xe2\x81")); len(out) != 0 {
		t.Errorf("a possible marker prefix was written instead of held: %q", out)
	}
}

// TestTheGameCanAskWhetherBashStillOwnsTheTerminal: the question the nudge asks
// before it speaks, and it has to be right in both directions. A student stuck at
// a prompt has to be nudged; a student reading `man ls` has to be left alone, and
// from inside the game the two look the same — no output, no Enter, no marker.
func TestTheGameCanAskWhetherBashStillOwnsTheTerminal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the foreground process group is read from the pty: Linux only")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on PATH")
	}
	s, err := newSession(t.TempDir(), newUI(io.Discard))
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	defer s.close()
	if !s.atItsOwnPrompt() {
		t.Fatal("bash should own the terminal when it is waiting for input")
	}

	// A child takes the terminal, and the question has to change answer while it
	// runs: this is the `man ls` / `sleep 60` / `cat` case the nudge must not
	// interrupt.
	s.keys([]byte("sleep 5\n"))
	gone := false
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
		if !s.atItsOwnPrompt() {
			gone = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !gone {
		t.Fatal("a running command should own the terminal, not bash")
	}

	// And bash takes it back, or the nudge would stay silent for the rest of the
	// level after one Ctrl-C.
	s.keys([]byte{0x03})
	back := false
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
		if s.atItsOwnPrompt() {
			back = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !back {
		t.Error("bash did not take the terminal back after Ctrl-C")
	}
}
