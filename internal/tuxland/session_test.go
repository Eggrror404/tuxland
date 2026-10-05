package tuxland

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
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

// TestCtrlDStillLeavesWhateverTheEnvironmentSays: the shell the game spawns
// inherits an environment the player never saw — a container image, a distro
// default, someone's .bashrc — and a key the game documents as a way out has to
// work anyway. `ignoreeof` is the one that bites: set, and Ctrl-D at an empty
// prompt answers "Use \"exit\" to leave the shell." forever, so the level cannot
// be left and the toolbox's own Ctrl-D card becomes a lie.
//
// Two checks, because either alone would pass by accident. The stripping is the
// pure function, and an empty value is not enough — bash reads an empty
// `ignoreeof` as still set, so the entry has to be gone. The second half asks the
// real shell, since what actually matters is not the variable list but whether
// the key works.
func TestCtrlDStillLeavesWhateverTheEnvironmentSays(t *testing.T) {
	// bash honours two spellings — the exported `IGNOREEOF` and the `ignoreeof` a
	// .bashrc sets — and an empty value counts as set, so both forms have to go. A
	// name that merely resembles one is left alone.
	env := []string{
		"PATH=/bin", "ignoreeof=1", "IGNOREEOF=", "IgnoreEof=2",
		"TUXLAND_TEST=keep", "TERM=xterm",
	}
	got := withoutEnv(env, "ignoreeof")
	want := []string{"PATH=/bin", "TUXLAND_TEST=keep", "TERM=xterm"}
	if len(got) != len(want) {
		t.Fatalf("withoutEnv kept %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("withoutEnv gave %q, want %q", got, want)
		}
	}
	// The caller's slice is left alone: os.Environ() is a fresh slice each time,
	// but a test that mutated it would hide that this returns a new one.
	if env[1] != "ignoreeof=1" || len(env) != 6 {
		t.Fatalf("withoutEnv changed the slice it was given: %q", env)
	}

	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on PATH")
	}
	// Only worth running where the environment actually does set it: the point is
	// that the game survives it, not that every machine has it.
	set := false
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.EqualFold(name, "ignoreeof") {
			set = true
		}
	}
	if !set {
		t.Skip("this environment does not set ignoreeof, so there is nothing to survive")
	}
	s, err := newSession(t.TempDir(), newUI(io.Discard))
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	defer s.close()
	s.keys([]byte("\x04")) // Ctrl-D at an empty prompt: leave
	select {
	case <-s.wait:
	case <-time.After(10 * time.Second):
		t.Error("Ctrl-D did not end the shell, and an inherited ignoreeof is the likeliest reason")
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

// TestAClosedSessionLeavesNothingWatchingIt: a level is a new session each time, and
// the winsize watcher is the one goroutine that waits on something the pty closing
// does not end. Left behind, it keeps a signal handler registered and keeps resizing
// a closed pty — so three levels in, every window drag costs three resizes to
// nothing. Sessions are opened and closed in a loop and the goroutine count has to
// come back down.
func TestAClosedSessionLeavesNothingWatchingIt(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on PATH")
	}
	// Warm up first: the first session pays for lazily-started runtime machinery
	// (the timer for a bash that has not exited yet, notably) that is not a leak.
	warm, err := newSession(t.TempDir(), newUI(io.Discard))
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	warm.close()
	time.Sleep(250 * time.Millisecond)

	before := runtime.NumGoroutine()
	for i := 0; i < 5; i++ {
		s, err := newSession(t.TempDir(), newUI(io.Discard))
		if err != nil {
			t.Fatalf("newSession: %v", err)
		}
		s.close()
	}
	var after int
	for until := time.Now().Add(5 * time.Second); time.Now().Before(until); {
		if after = runtime.NumGoroutine(); after <= before {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("five closed sessions left %d goroutines running, was %d before them", after-before, before)
}
