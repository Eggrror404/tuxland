package tuxland

import (
	"os"
	"path/filepath"
	"testing"
)

// $LINUXLAB_ROOT is the one variable that aims the game's recursive delete, so
// the folders it must never be asked for are refused by name. The dangerous ones
// are the plausible ones: `.` (the folder you happen to be standing in), `/`, and
// your own home — and a quoted `~` or `~/labs`, which the shell never expanded and
// the game must not treat as a folder name.
func TestTheLabRootRefusesAFolderItWouldWipe(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for _, never := range []string{
		".", "./", string(filepath.Separator), home, "~", filepath.Clean(cwd),
		"~root/labs", // another account's home
		"/etc/hostname",
	} {
		t.Setenv("LINUXLAB_ROOT", never)
		if root, err := labRoot(); err == nil {
			t.Errorf("LINUXLAB_ROOT=%s was accepted as %s", never, root)
		}
	}
}

// And the other half: a folder of its own is taken as asked, `~` and `~/…` are
// expanded, and what comes back is absolute — so a playground is never named
// relative to wherever the game happened to be started.
func TestTheLabRootTakesAFolderOfItsOwn(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	dir := t.TempDir()
	for in, want := range map[string]string{
		dir:                                dir,
		filepath.Join(dir, "not-made-yet"): filepath.Join(dir, "not-made-yet"),
		"~/linux-lab-may-not-exist":        filepath.Join(home, "linux-lab-may-not-exist"),
		filepath.Join(dir, "..", filepath.Base(dir)): dir,
	} {
		t.Setenv("LINUXLAB_ROOT", in)
		got, err := labRoot()
		if err != nil {
			t.Errorf("LINUXLAB_ROOT=%s was refused: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("LINUXLAB_ROOT=%s came back as %s, want %s", in, got, want)
		}
	}
	// Nothing is created by asking: only scaffolding makes a playground.
	if _, err := os.Stat(filepath.Join(home, "linux-lab-may-not-exist")); err == nil {
		t.Error("labRoot created a folder just by being asked")
	}
}

// The default needs no variable at all, and it is a folder with a name of its own
// so the one thing the game wipes is obviously the game's.
func TestTheLabRootDefaultsToALabFolder(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	t.Setenv("LINUXLAB_ROOT", "")
	got, err := labRoot()
	if err != nil {
		t.Fatalf("labRoot: %v", err)
	}
	if want := filepath.Join(home, "linux-lab"); got != want {
		t.Errorf("the default lab root is %s, want %s", got, want)
	}
}
