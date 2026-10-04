package tuxland

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// A check inspects the real filesystem inside a level's playground and reports
// whether the step's goal is met. That is the *only* way the game grades a
// task: the game never looks at what the student typed, only at what landed on
// disk. Every check is written against a playground-relative path.
type check func(dir string) bool

// at joins a playground-relative path.
func at(dir, name string) string { return filepath.Join(dir, name) }

// exists is the plainest check there is: the path is there.
func exists(name string) check {
	return func(dir string) bool { _, err := os.Stat(at(dir, name)); return err == nil }
}

// isFile: the path is there and it is a regular file.
func isFile(name string) check {
	return func(dir string) bool {
		fi, err := os.Stat(at(dir, name))
		return err == nil && fi.Mode().IsRegular()
	}
}

// isDir: the path is there and it is a folder.
func isDir(name string) check {
	return func(dir string) bool {
		fi, err := os.Stat(at(dir, name))
		return err == nil && fi.IsDir()
	}
}

// isGone: nothing is there any more (the `rm` step).
func isGone(name string) check {
	return func(dir string) bool {
		_, err := os.Stat(at(dir, name))
		return errors.Is(err, fs.ErrNotExist)
	}
}

// contentIs: the file's text is exactly want (trailing space ignored).
func contentIs(name, want string) check {
	return func(dir string) bool {
		got, err := readFile(at(dir, name))
		return err == nil && strings.TrimSpace(got) == strings.TrimSpace(want)
	}
}

// contentContains: the file's text mentions sub. Forgiving on purpose — the
// point of the step is the command, not the exact formatting.
func contentContains(name, sub string) check {
	return func(dir string) bool {
		got, err := readFile(at(dir, name))
		return err == nil && strings.Contains(got, sub)
	}
}

// lineCountIs: the file has exactly n lines.
func lineCountIs(name string, n int) check {
	return func(dir string) bool {
		got, err := readFile(at(dir, name))
		if err != nil {
			return false
		}
		got = strings.TrimRight(got, "\n")
		if got == "" && n > 0 {
			return false
		}
		return len(strings.Split(got, "\n")) == n
	}
}

// modeIs: the permission bits are exactly want (0o600, 0o700, …).
func modeIs(name string, want os.FileMode) check {
	return func(dir string) bool {
		fi, err := os.Stat(at(dir, name))
		return err == nil && fi.Mode().Perm() == want.Perm()
	}
}

// isExecutable: at least one execute bit is on (what `chmod +x` does).
func isExecutable(name string) check {
	return func(dir string) bool {
		fi, err := os.Stat(at(dir, name))
		return err == nil && fi.Mode().Perm()&0o111 != 0
	}
}

// all / any compose checks (e.g. "gone from here, and in there instead").
func all(cs ...check) check {
	return func(dir string) bool {
		for _, c := range cs {
			if !c(dir) {
				return false
			}
		}
		return true
	}
}

func any(cs ...check) check {
	return func(dir string) bool {
		for _, c := range cs {
			if c(dir) {
				return true
			}
		}
		return false
	}
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
