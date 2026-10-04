package tuxland

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// progress is what the game remembers between runs: which levels the student has
// finished. It is a convenience and nothing more — the game never *needs* it, so
// every failure to read or write it is shrugged off rather than reported, and a
// lost file costs the student a suggested starting level, not their work.
//
// It lives in the playground root beside the level folders, which is deliberate:
// one folder to look at, and `rm -rf ~/linux-lab` still resets everything the game
// has ever known about you.
type progress struct {
	done      map[int]bool
	extraDone bool // the habits toolbox has no number, so it is remembered apart
}

// progressFile is the name inside the lab root. Plain .json on purpose: a
// student who wants to see their own state can read it, and hand-edit it back
// (nonsense in it is ignored, see loadProgress).
const progressFile = "progress.json"

func progressPath() (string, error) {
	root, err := labRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, progressFile), nil
}

// progressDoc is the file's shape. The field is a list rather than an object so
// the file stays obvious to read, and it grows to whatever the game adds.
type progressDoc struct {
	Done  []int `json:"done"`
	Extra bool  `json:"extra,omitempty"` // the habits toolbox, finished or not
}

// lenientDoc is the same shape for *reading*, with the list left untyped (the
// builtin `any` would be shadowed by a field called Done). The file is meant to be
// hand-editable, and one nonsense element — a stray word, a half-finished edit —
// should cost that element and not the whole record. Extra stays raw for the same
// reason: a hand-typed `"extra": "yes"` must not invalidate the whole file.
type lenientDoc struct {
	Done  []interface{}   `json:"done"`
	Extra json.RawMessage `json:"extra"`
}

func newProgress() progress { return progress{done: map[int]bool{}} }

// loadProgress reads the file if it is there. Missing, unreadable, truncated or
// hand-mangled all mean the same thing to a game that can do without it: no
// levels finished yet. A number that is not a level is dropped, and duplicates
// collapse, so a student who edits the file cannot talk the menu into a level
// that does not exist.
func loadProgress() progress {
	p := newProgress()
	path, err := progressPath()
	if err != nil {
		return p
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	var doc lenientDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return p
	}
	for _, v := range doc.Done {
		f, ok := v.(float64) // every JSON number decodes as one
		if !ok || f != float64(int(f)) {
			continue
		}
		if n := int(f); n >= 1 && n <= NumLevels() {
			p.done[n] = true
		}
	}
	if len(doc.Extra) > 0 && string(doc.Extra) == "true" {
		p.extraDone = true
	}
	return p
}

// save writes the file through a temporary name in the same folder and renames
// it over the top, so a run that dies mid-write leaves the old progress rather
// than a half-written file. Like loading, a failure is not worth interrupting a
// session over: the levels are still there, the game just forgets next time.
func (p progress) save() error {
	path, err := progressPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	nums := make([]int, 0, len(p.done))
	for n := range p.done {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	raw, err := json.MarshalIndent(progressDoc{Done: nums, Extra: p.extraDone}, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("tuxland: cannot write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("tuxland: cannot write %s: %w", path, err)
	}
	return nil
}

// markDone records a finished level. The caller saves; the game saves once, at
// the moment the level's flag was found, so an interrupted run still counts.
func (p progress) markDone(n int) { p.done[n] = true }

func (p progress) isDone(n int) bool { return p.done[n] }

// markExtraDone records the habits toolbox. The extra has no number to sit in
// the done list, so it lives beside it — and is never "the next level".
//
// A pointer receiver, unlike markDone: done is a map and a copy still writes to
// the same map, but extraDone is a plain bool and a copy's change would evaporate.
func (p *progress) markExtraDone() { p.extraDone = true }

func (p progress) isExtraDone() bool { return p.extraDone }

func (p progress) count() int { return len(p.done) }

// next is the level to put the cursor on: the first one not finished, so a
// student who quits halfway comes back to where they stopped. A student who has
// done all of them is offered level 1 again — the game is a practice tool, and
// redoing a level is a normal thing to want. The extra is never next: it is
// something a student walks to on purpose, not a step in the sequence.
func (p progress) next() int {
	numbered := numberedLevels()
	for _, lv := range numbered {
		if !p.done[lv.num] {
			return lv.num
		}
	}
	return numbered[0].num
}
