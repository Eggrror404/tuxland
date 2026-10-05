package tuxland

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// labRoot is where every level's playground lives: ~/linux-lab by default, or
// $LINUXLAB_ROOT so the game can be pointed somewhere harmless (tests, demos).
// Nothing outside this root is ever touched.
func labRoot() (string, error) {
	if dir := os.Getenv("LINUXLAB_ROOT"); dir != "" {
		return safeRoot(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("tuxland: no home directory: %w", err)
	}
	return defaultRoot(home), nil
}

// defaultRoot is the folder the game asks for when nothing is set. ~/linux-lab:
// a name of its own, so the one thing the game wipes is obviously the game's.
func defaultRoot(home string) string { return filepath.Join(home, "linux-lab") }

// safeRoot is $LINUXLAB_ROOT's answer, and the guard the variable deserves. Every
// run wipes and rebuilds level-* inside this folder, so a root pointed at the
// wrong place is a recursive delete aimed at somebody's work — and this is a
// workshop tool, handed to students who type what they are told. So a pasted
// `~` or `~/labs` is expanded (a quoted path is the obvious mistake, and a
// literal `~` folder is not somewhere to put a playground), and the three
// folders that must never be one — the filesystem root, the home folder itself,
// and the directory the game was started in, which is what `LINUXLAB_ROOT=.`
// means — are refused by name rather than discovered afterwards.
func safeRoot(dir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("tuxland: no home directory: %w", err)
	}
	switch {
	case dir == "~":
		dir = home
	case strings.HasPrefix(dir, "~/"):
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~/"))
	case strings.HasPrefix(dir, "~"):
		// ~someone's home. The game's own home is the only one it may guess at.
		return "", fmt.Errorf("tuxland: LINUXLAB_ROOT %s is another account's home — spell the path out, "+
			"or start it with ~/ for your own", dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("tuxland: LINUXLAB_ROOT %q is not a usable path: %w", dir, err)
	}
	// A symlink only matters once it exists, and the playground is the one folder
	// the game makes itself — so resolve as much of the path as is already there
	// and keep the missing tail as written. What comes back stays the way the
	// student spelled it, so a `~` in there still shows as `~` in the banner.
	target := resolve(abs)
	if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
		return "", fmt.Errorf("tuxland: LINUXLAB_ROOT %s is a file, not a folder", abs)
	}
	if cwd, err := os.Getwd(); err == nil {
		for _, never := range []string{string(filepath.Separator), home, cwd} {
			// Both sides resolved, or this guard is a name comparison a symlink walks
			// past: network homes are nearly always reached through one, and
			// `LINUXLAB_ROOT=$HOME` behind it used to be taken as a folder of its own.
			if target == resolve(never) {
				return "", fmt.Errorf("tuxland: refusing to use %s as LINUXLAB_ROOT — every run wipes and "+
					"rebuilds the level folders inside it. Point it at a folder of its own, like %s",
					abs, defaultRoot(home))
			}
		}
	}
	return abs, nil
}

// resolve walks the symlinks in as much of path as exists and keeps the rest as
// written. filepath.EvalSymlinks gives up on a path whose tail is missing, but
// that is the normal state of a playground, and the home folder above it is
// still a symlink that needs seeing through.
func resolve(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(real)
	}
	parent, base := filepath.Split(path)
	if parent == "" || parent == path {
		return filepath.Clean(path) // nothing above to look at
	}
	return filepath.Join(resolve(filepath.Clean(parent)), base)
}

// levelDirName is the playground folder a level lives in. The extra has no
// number, so it keeps a folder of its own instead of `level-00-…`.
func levelDirName(lv *level) string {
	if lv.extra {
		return "level-extra-" + lv.name
	}
	return fmt.Sprintf("level-%02d-%s", lv.num, lv.name)
}

// scaffoldLevel wipes and rebuilds a level's playground, so every run — and
// every `-level N` rehearsal — starts from the same clean state. That is the
// whole reset story: re-run the game and your level is fresh again.
func scaffoldLevel(lv *level) (string, error) {
	root, err := labRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, levelDirName(lv))
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("tuxland: cannot clear %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("tuxland: cannot create %s: %w", dir, err)
	}
	if err := lv.scaffold(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// put writes a scaffold file, creating the folder chain above it if needed.
func put(dir, name, content string) error { return putMode(dir, name, 0o644, content) }

// putMode is put with an explicit mode, for the files whose permissions a step
// is graded on.
func putMode(dir, name string, mode os.FileMode, content string) error {
	path := filepath.Join(dir, name)
	if parent := filepath.Dir(path); parent != dir {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return err
	}
	// Be explicit: the umask must not decide a mode a step is graded on.
	return os.Chmod(path, mode)
}

// putDir makes a scaffold folder with an exact mode.
func putDir(dir, name string, mode os.FileMode) error {
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// tilde shows a path as ~/… so banners stay short and the home folder is not
// spelled out on someone's screen during a demo.
func tilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join("~", rel)
}
