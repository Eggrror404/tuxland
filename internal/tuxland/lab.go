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
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("tuxland: no home directory: %w", err)
	}
	return filepath.Join(home, "linux-lab"), nil
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
