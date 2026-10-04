package tuxland

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// Nothing in this file looks at what a level *says*. These are the rules the game
// itself depends on — the ones that, if broken, make a level unplayable or unfair
// whatever the wording is — and they are checked against every step of every
// level, so a level added tomorrow is covered without anyone remembering.
//
// Content has its own layer and it is not here. `replay_test.go` plays each level
// through the real binary on a real pty, which is what catches a fixture the
// checks no longer match; `zz_dump_test.go` prints the content to read. A test that
// asserts "today.txt has three lines" or "the hunt's script has fifty of them"
// only records what one author decided on one day, and it fails the next person who
// rewords a card or renames a helper. So: structure here, content there.

// scaffoldFor builds a level's playground the way the game does and leaves it
// *sealed*, because the seal is part of what a level asserts: level 6's vault
// task checks a mode, and opening the vault to make a test convenient would
// satisfy it before the student arrived.
//
// The directory is not a t.TempDir for the same reason. TempDir's cleanup is
// registered before this one and cleanups run last-in-first-out, so it would run
// *after* ours — by which time we would have removed the tree it is trying to
// delete, and it would report a "permission denied" that has nothing to do with
// the level. Hence a plain MkdirTemp with our own cleanup.
func scaffoldFor(t *testing.T, lv *level) string {
	t.Helper()
	root, err := os.MkdirTemp("", "tuxland-level-")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, levelDirName(lv))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := lv.scaffold(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		unseal(root)
		_ = os.RemoveAll(root)
	})
	return dir
}

// unseal re-opens every folder in root that has no owner `x`.
//
// Level 6 seals `secret/` at 0600 on purpose: a folder without `x` turns away
// even the student who owns it, which is the whole level. Two test-only needs
// then cannot get in — `WalkDir` cannot descend, so the flag's word reads as
// absent, and `TempDir`'s own cleanup cannot delete the tree. Neither is a fault
// in the level, so the seal comes off where a test needs to see inside.
//
// Only owner `x` is touched, never the other bits, and never at grading time: a
// test that asserts a mode calls this *after* it has read the mode it wanted.
func unseal(root string) {
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if mode := fi.Mode().Perm(); mode&0o100 == 0 {
			_ = os.Chmod(p, mode|0o100)
		}
		return nil
	})
}

func eachStep(fn func(lv *level, st *step)) {
	for _, lv := range levels {
		for _, sec := range lv.sections {
			for i := range sec.steps {
				fn(lv, &sec.steps[i])
			}
		}
	}
}

func eachFlagStep(fn func(lv *level, st *step)) {
	eachStep(func(lv *level, st *step) {
		if st.kind == kindFlag {
			fn(lv, st)
		}
	})
}

// A task must start out unfulfilled: if the scaffold already satisfies a check,
// the game would congratulate a student who did nothing.
//
// Each step is checked against the state the student actually meets — scaffold
// plus the flags planted by the steps before it.
func TestNoTaskIsFree(t *testing.T) {
	empty := t.TempDir() // nothing in here at all
	for _, lv := range levels {
		dir := scaffoldFor(t, lv)
		for _, sec := range lv.sections {
			for i := range sec.steps {
				st := &sec.steps[i]
				// An `rm` step asks for something to be *gone*, which only means
				// something because the step before it created it — an empty
				// folder tells us nothing about those, so skip them.
				if st.kind == kindTask && !st.check(empty) && st.check(dir) {
					t.Errorf("level %d: %q is already done when the student arrives", lv.num, st.goal)
				}
				if err := plantAll(dir, st.plant); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

// The game must never print a token itself: the flag has to be found, not read
// out of a hint. This holds for any wording, which is why it is a rule about the
// step rather than about any card.
func TestTokensStayOutOfTheProse(t *testing.T) {
	eachStep(func(lv *level, st *step) {
		prose := strings.Join([]string{st.goal, st.note, st.done, strings.Join(st.hints, "\n")}, "\n")
		if st.token != "" && strings.Contains(prose, st.token) {
			t.Errorf("level %d: the token leaks into the card %q", lv.num, st.goal)
		}
	})
}

// Hints and ✅ lines are automatic, so a step without them would leave the
// student with nothing to hear and nothing to celebrate. The kind decides what
// else it has to carry: a flag a token, a task a check.
func TestEveryStepIsComplete(t *testing.T) {
	eachStep(func(lv *level, st *step) {
		switch {
		case st.kind == kindFlag && st.token == "":
			t.Errorf("level %d: flag step %q has no token", lv.num, st.goal)
		case st.kind == kindTask && st.check == nil:
			t.Errorf("level %d: task %q has no check", lv.num, st.goal)
		case st.kind == kindInfo:
			// nothing to look for on disk
		}
		if len(st.hints) == 0 {
			t.Errorf("level %d: step %q has no hints (hints are automatic, so it needs some)", lv.num, st.goal)
		}
		if st.done == "" {
			t.Errorf("level %d: step %q has no ✅ line", lv.num, st.goal)
		}
	})
}

// A hunt the word is not in is a hunt the student cannot finish. One token per
// hunt, and the plant has to put it somewhere findable — unless the level says
// otherwise with `assembled`, which is the one case where the word is meant to be
// absent from the disk because the level builds it at runtime.
func TestFlagsArePlanted(t *testing.T) {
	seen := map[string]bool{}
	eachFlagStep(func(lv *level, st *step) {
		if seen[st.token] {
			t.Errorf("level %d: token %q is used twice", lv.num, st.token)
		}
		seen[st.token] = true
		dir := scaffoldFor(t, lv)
		if err := plantAll(dir, st.plant); err != nil {
			t.Fatal(err)
		}
		// Level 6's word is behind a vault the student has to open, so the walk
		// below cannot descend until something opens it. Nothing here grades a
		// mode, so lifting the seal is safe.
		unseal(dir)
		if st.assembled {
			if treeContains(dir, st.token) {
				t.Errorf("level %d: %q is on the disk verbatim — an assembled hunt must not be readable", lv.num, st.token)
			}
			return
		}
		if !treeContains(dir, st.token) {
			t.Errorf("level %d: %q is nowhere in the playground", lv.num, st.token)
		}
	})
}

// A plant list runs in order, and `putMode` writes a file *through* its parent
// folder. So a folder planted without the owner `x` bit — a sealed vault —
// cannot have its contents planted after it: the write is refused, the
// scaffold returns an error, and level 6 comes up half-built with a vault that
// has no flag in it. Its contents are planted first so `MkdirAll` leaves the
// folder writable long enough to fill, and the `putDir` afterwards closes it.
//
// This is structure, not content: it says nothing about *what* is planted, only
// about an order that either works or cannot. Level 6 got this wrong the first
// time and the replay caught it, which is the argument for checking it here too.
func TestAPlantInsideASealedFolderComesFirst(t *testing.T) {
	check := func(where string, ps []plant) {
		// Folders already planted *and closed*: planted without the owner x bit,
		// which is what a later write cannot get through.
		closed := map[string]os.FileMode{}
		for _, p := range ps {
			if parent := path.Dir(p.at); parent != "." && parent != "/" {
				if mode, ok := closed[parent]; ok {
					t.Errorf("%s: %q is planted after its folder %q was planted at %#o, "+
						"which has no owner x — the write cannot get through it",
						where, p.at, parent, mode)
				}
			}
			if p.dir && p.modeOr(0o755)&0o100 == 0 {
				closed[p.at] = p.modeOr(0o755)
			}
		}
	}
	for _, lv := range levels {
		check(lv.name+"/seed", lv.seed)
		eachStep(func(lv *level, st *step) { check(lv.name+"/step", st.plant) })
	}
}

// Every numbered level has to end in a hunt with its own token, because the menu's
// digit keys, `-level`, the progress file's range check and the finale's banner all
// read the count, and a level whose token is missing or shared cannot be finished.
// The count is derived, not asserted: pinning it to a number here would make adding
// a seventh level a test edit, which is the wrong reason to touch a test.
func TestEveryLevelEndsInItsOwnHunt(t *testing.T) {
	seen := map[string]bool{}
	for _, lv := range numberedLevels() {
		flags := 0
		for _, sec := range lv.sections {
			for i := range sec.steps {
				st := &sec.steps[i]
				if st.kind != kindFlag {
					continue
				}
				flags++
				if seen[st.token] {
					t.Errorf("level %d: token %q is used twice", lv.num, st.token)
				}
				seen[st.token] = true
			}
		}
		if flags != 1 {
			t.Errorf("level %d (%s) has %d flag steps, want exactly 1", lv.num, lv.name, flags)
		}
	}
	if len(seen) != NumLevels() {
		t.Errorf("%d distinct tokens for %d numbered levels", len(seen), NumLevels())
	}
}

// The sequence the menu's digit keys index into has to be the numbering the levels
// carry, and the toolbox has to stay outside it: it is reached with ↓, it is never
// "the last level", and no digit selects it.
func TestLevelNumbers(t *testing.T) {
	for i, lv := range numberedLevels() {
		if lv.num != i+1 {
			t.Errorf("level %d is numbered %d", i+1, lv.num)
		}
		if levelDirName(lv) == "" || lv.name == "" || lv.title == "" {
			t.Errorf("level %d is missing name/title", lv.num)
		}
		if len(lv.sections) == 0 {
			t.Errorf("level %d has no sections", lv.num)
		}
		if lv.isExtra() {
			t.Errorf("level %d is marked extra, but it sits in the sequence", lv.num)
		}
	}
	// The extra is the habits toolbox: no number, last in the list, menu-only,
	// and never "the last level".
	tool := extraLevel()
	if tool != levels[len(levels)-1] {
		t.Error("the extra is not last in levels")
	}
	if !tool.isExtra() || tool.num != 0 {
		t.Errorf("the extra is not marked extra/numberless: num=%d extra=%v", tool.num, tool.extra)
	}
	if tool.isLast() {
		t.Error("the extra must not be the last numbered level")
	}
	if last := numberedLevels()[NumLevels()-1]; !last.isLast() {
		t.Errorf("level %d should be the last one", last.num)
	}
	if levelDirName(tool) == "" {
		t.Error("the extra has no playground folder name")
	}
}

// treeContains is whether sub is anywhere in the playground, in a name or in a
// file. A name counts: a level is allowed to hide its word in a folder's name.
func treeContains(dir, sub string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if strings.Contains(d.Name(), sub) {
			found = true
			return nil
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(b), sub) {
			found = true
		}
		return nil
	})
	return found
}
