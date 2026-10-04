package tuxland

import (
	"bytes"
	"testing"
)

// session_test.go covers the prompt marker, which is the shell's own "you may answer
// now" and the one fact the game never guesses about. It is byte logic, so it is
// tested as byte logic rather than left to a 50-second wait on a pty.
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
