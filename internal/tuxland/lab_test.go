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

// The guard compares the root against the home folder and the working folder, so
// it has to compare like with like. Home folders reached through a symlink — a
// network home, a course share, `/home/student/14/…` — are the common case, not
// the exotic one, and `LINUXLAB_ROOT=$HOME` behind such a symlink used to sail
// straight through: the candidate was resolved, the two it must not equal were not.
func TestTheLabRootRefusesAFolderHiddenBehindASymlink(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "the-real-home")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(base, "a-nice-short-home")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}
	t.Setenv("HOME", link)

	// The same folder, reached two ways. Both are the home folder, so both are
	// refused, and the refusal has to name the real path — that is what the game
	// would have wiped.
	for _, never := range []string{link, real, "~", "~/"} {
		t.Setenv("LINUXLAB_ROOT", never)
		if root, err := labRoot(); err == nil {
			t.Errorf("LINUXLAB_ROOT=%s was accepted as %s", never, root)
		}
	}
	// A folder inside the home is still a folder of its own — the guard is about
	// the folder itself, not about how you got to it.
	t.Setenv("LINUXLAB_ROOT", filepath.Join(link, "linux-lab"))
	if got, err := labRoot(); err != nil {
		t.Errorf("LINUXLAB_ROOT=<home>/linux-lab was refused: %v", err)
	} else if want := filepath.Join(link, "linux-lab"); got != want {
		t.Errorf("LINUXLAB_ROOT=<home>/linux-lab came back as %s, want %s", got, want)
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
