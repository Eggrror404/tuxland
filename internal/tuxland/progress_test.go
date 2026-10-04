package tuxland

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// progressOf is a progress with the given levels finished, for the screens that
// draw one (the kickoff's ✅ gutter and its progress sentence have three states:
// none, some, all).
func progressOf(nums ...int) progress {
	p := newProgress()
	for _, n := range nums {
		p.markDone(n)
	}
	return p
}

func TestProgressIsRememberedBetweenRuns(t *testing.T) {
	lab := t.TempDir()
	t.Setenv("LINUXLAB_ROOT", lab)

	if got := loadProgress(); got.count() != 0 {
		t.Fatalf("a fresh lab has %d levels done, not 0", got.count())
	}
	progressOf(1, 2).save()
	if got := loadProgress(); !got.isDone(1) || !got.isDone(2) || got.isDone(3) {
		t.Errorf("after saving 1,2: 1=%v 2=%v 3=%v — want true, true, false",
			got.isDone(1), got.isDone(2), got.isDone(3))
	}
}

func TestTheFileIsWhereTheGameSaysItIs(t *testing.T) {
	lab := t.TempDir()
	t.Setenv("LINUXLAB_ROOT", lab)
	if err := progressOf(3).save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lab, progressFile)); err != nil {
		t.Errorf("no %s in the lab root: %v", progressFile, err)
	}
	// Beside the level folders, not inside one: scaffolding wipes a level's folder
	// on every run, and the record has to survive that.
	if _, err := os.Stat(filepath.Join(lab, "level-01-navigation")); !os.IsNotExist(err) {
		t.Errorf("saving progress created a level folder, which is not its business")
	}
}

// The file is a convenience, never a dependency: everything that can go wrong
// with it is shrugged off, because the game can do without it and a student
// cannot be asked to debug a JSON file mid-level.
func TestBadProgressIsIgnoredNotFatal(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          int // levels the game should believe are done
	}{
		{"missing", "", 0},
		{"truncated", `{"done": [1, 2`, 0},
		{"not json", "hello, I am a text file", 0},
		{"wrong shape", `{"finished": true}`, 0},
		{"empty", `{}`, 0},
		{"null", `null`, 0},
		{"nonsense numbers", `{"done": [0, 7, -3, 99, "two", 2.5, 4]}`, 1}, // just 4
		{"duplicates", `{"done": [2, 2, 2]}`, 1},
		{"extra fields", `{"done": [1], "note": "hand-edited", "v": 9}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lab := t.TempDir()
			t.Setenv("LINUXLAB_ROOT", lab)
			if tc.content != "" {
				if err := os.WriteFile(filepath.Join(lab, progressFile), []byte(tc.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := loadProgress(); got.count() != tc.want {
				t.Errorf("%s: %d levels done, want %d", tc.content, got.count(), tc.want)
			}
		})
	}
}

// The write goes through a temporary name and a rename, so a run that dies
// mid-write leaves the old progress rather than half a file — and never leaves
// the temporary behind for the next run to trip over.
func TestTheProgressFileIsNeverLeftHalfWritten(t *testing.T) {
	lab := t.TempDir()
	t.Setenv("LINUXLAB_ROOT", lab)

	if err := progressOf(1, 2).save(); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := progressOf(1, 2, 3, 4).save(); err != nil {
		t.Fatalf("second save: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(lab, progressFile))
	if err != nil {
		t.Fatal(err)
	}
	var doc progressDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the file is not valid JSON after two saves: %v\n%s", err, raw)
	}
	if len(doc.Done) != 4 {
		t.Errorf("done = %v, want four levels", doc.Done)
	}
	// Sorted, so the file is the same every time it is written: a student who
	// looks at it, or diffs it, sees a change only when something changed.
	if doc.Done[0] != 1 || doc.Done[1] != 2 || doc.Done[2] != 3 || doc.Done[3] != 4 {
		t.Errorf("done = %v, want it in level order", doc.Done)
	}
	entries, err := os.ReadDir(lab)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
}

// next is what the menu points at: where the student stopped, or level 1 once
// there is nothing left to finish.
func TestNextOffersTheFirstUnfinishedLevel(t *testing.T) {
	for _, tc := range []struct {
		done []int
		want int
	}{
		{nil, 1},
		{[]int{1}, 2},
		{[]int{1, 2, 3}, 4},
		{[]int{2}, 1},                // a gap at the front: the first one is still open
		{[]int{1, 2, 3, 4, 5, 6}, 1}, // all done: everything is open to replay
	} {
		p := progressOf(tc.done...)
		if got := p.next(); got != tc.want {
			t.Errorf("with %v done, next is level %d, want %d", tc.done, got, tc.want)
		}
	}
}

// The lab root does not exist on a first run, so saving has to make it. And a
// lab root that cannot be written to at all — a read-only home, a full disk — has
// to be an error the caller can ignore, never a panic in a student's face.
func TestProgressIsSavedIntoALabRootThatDoesNotExistYet(t *testing.T) {
	t.Setenv("LINUXLAB_ROOT", filepath.Join(t.TempDir(), "no-such-lab"))
	if err := progressOf(1).save(); err != nil {
		t.Fatalf("save into a lab root that does not exist yet: %v", err)
	}
	if got := loadProgress(); !got.isDone(1) {
		t.Errorf("saved level 1 did not come back")
	}
}

func TestAnUnwritableLabIsAnErrorNotACrash(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes to a read-only directory anyway")
	}
	lab := t.TempDir()
	if err := os.Chmod(lab, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lab, 0o755) })
	t.Setenv("LINUXLAB_ROOT", lab)

	if err := progressOf(1).save(); err == nil {
		t.Errorf("saving into a read-only lab root reported success")
	}
	// Whatever happened, the game can still read a level list and carry on.
	if got := loadProgress(); got.count() != 0 {
		t.Errorf("a failed save left %d levels done behind", got.count())
	}
}

// levelComplete is where a level becomes a memory: the flag is found, the
// celebration goes up, and the file records it then and there — a run cut short
// after that instant still counts the level. That moment is the whole feature,
// so it gets a direct test of its own, without a pty or a shell.
func TestFinishingALevelWritesTheFile(t *testing.T) {
	lab := t.TempDir()
	t.Setenv("LINUXLAB_ROOT", lab)

	var buf bytes.Buffer
	p := newProgress()
	g := &game{ui: newUI(&buf), prog: &p}
	g.levelComplete(levelByNum(1))

	if _, err := os.Stat(filepath.Join(lab, progressFile)); err != nil {
		t.Errorf("finishing level 1 did not write %s: %v", progressFile, err)
	}
	got := loadProgress()
	if !got.isDone(1) {
		t.Errorf("finishing level 1 did not mark it done")
	}
	// And that record is what the menu will point at next time.
	if next := got.next(); next != 2 {
		t.Errorf("after finishing level 1, the menu offers level %d, want 2", next)
	}
}

// The extra is remembered like a level but is not *a level*: the count and the
// menu's cursor are about the five numbered ones, so finishing the toolbox must
// not pretend a level is done or move "next up" off the sequence.
func TestTheExtraIsRememberedButNotCounted(t *testing.T) {
	lab := t.TempDir()
	t.Setenv("LINUXLAB_ROOT", lab)

	p := progressOf(1, 2, 3, 4)
	p.markExtraDone()
	if err := p.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	got := loadProgress()
	if !got.isExtraDone() {
		t.Error("the extra's ✅ did not come back")
	}
	if got.count() != 4 {
		t.Errorf("the extra counted as a level: %d done, want 4", got.count())
	}
	if next := got.next(); next != 5 {
		t.Errorf("the extra moved the menu: next is level %d, want 5", next)
	}
	raw, err := os.ReadFile(filepath.Join(lab, progressFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"extra": true`) {
		t.Errorf("the file does not record the extra: %s", raw)
	}
}

// A hand-typed extra flag that is not the boolean is ignored, exactly as a
// junk element in the done list is: the file is meant to survive being edited.
func TestAHandEditedExtraFlagIsIgnoredWhenItIsNotTrue(t *testing.T) {
	lab := t.TempDir()
	t.Setenv("LINUXLAB_ROOT", lab)
	if err := os.WriteFile(filepath.Join(lab, progressFile), []byte(`{"done": [1], "extra": "yes"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadProgress()
	if got.isExtraDone() {
		t.Error(`"extra": "yes" was read as finished`)
	}
	if !got.isDone(1) || got.count() != 1 {
		t.Errorf("the junk extra flag cost the done list: count=%d done1=%v", got.count(), got.isDone(1))
	}
}
