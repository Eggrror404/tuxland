package tuxland

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// needs are the programs a level's replay types at, so a machine missing one skips
// that level instead of failing the suite. Level 3 feeds `bc`, the game's one
// non-base dependency. There is no test of `bc` itself — it is a runtime
// dependency, so its answers are its business, and the replay runs the real thing
// (DESIGN.md §8).
var needs = map[int][]string{
	3: {"bc"},
}

// A whole level, typed. levels_test.go checks that the content obeys its rules and
// pty_test.go checks that the opening of a session is drawn where it should be, but
// neither finishes a level, and finishing one is the only thing that exercises the
// whole chain — every card's goal, the artifact each task checks for on the real
// disk, and the flag's scan of the screen. The two rules that make it trustworthy
// rather than decorative are in DESIGN.md §8.
//
// So each level here is a script: what a student types, at which card. The
// commands are the ones the cards ask for, which is the point — if a card's goal
// stops matching what actually satisfies it, the replay stalls on that card and
// says so, which is a far better failure than a test that still passes.
//
// This is also the tool to reach for when changing level content. To watch one:
//
//	TUXLAND_REPLAY=5 go test ./internal/tuxland -run TestALevelCanBeFinished -v
//
// and to watch the screen of every level, pass TUXLAND_DUMP=1 with it.

// move is one card, and what a student types at it.
//
// at is a fragment of the card's goal, matched anywhere on the screen — a
// fragment rather than the whole goal because goals wrap, and the first row of a
// wrapped goal is not the goal.
//
// type is the typing, in order, one command per entry and an empty string for a
// bare Enter. The game's own rules for how many that is:
//   - an info card runs the command, then takes a second Enter as the "I've read
//     it" tap, so a card with a command is two entries
//   - a card with nothing to run (the "in one line" beats) is one bare Enter
//   - a task card is one entry: the game checks the disk when the line comes back
type move struct {
	at    string
	type_ []string
}

// These four constructors mirror the four shapes a step takes — "look (info), do
// (task) or find (flag)" — because how many Enters a card wants is a property of
// its kind, not of the card, and getting it wrong is what makes a replay stall on
// a card that is doing exactly what it should.

// read is an info card with a command: run it, then tap Enter to say the output has
// been read.
func read(at, cmd string) move {
	return move{at: at, type_: []string{cmd, ""}}
}

// tap is a card with nothing to run — the "in one line" beats, which show a line
// and ask to be read. One bare Enter.
func tap(at string) move {
	return move{at: at, type_: []string{""}}
}

// fix is a task card: type the command that satisfies it, and the game looks at the
// disk when the line comes back.
func fix(at, cmd string) move {
	return move{at: at, type_: []string{cmd}}
}

// hunt is the flag hunt. The commands are whatever it takes to put the word on
// screen, which for every level is the shortest honest route and nothing more.
func hunt(at string, cmds ...string) move {
	return move{at: at, type_: cmds}
}

// replays is the whole game, one script per numbered level. Level 1's flag is in
// a folder's *name*, so `ls` is all it takes; level 6's is behind a folder the
// student locked themselves, so it is `cd` and a read.
var replays = map[int][]move{
	1: {
		read("ask where you are", "pwd"),
		// Two of level 1's cards ask for two commands, and the second Enter that
		// would have been the "I've read it" tap is the second command instead.
		move{at: "step into notes/", type_: []string{"cd notes", "pwd"}},
		move{at: "step back out", type_: []string{"cd ..", "pwd"}},
		read("see what's in this folder", "ls"),
		read("see the hidden names too", "ls -a"),
		read("see sizes, dates and permissions", "ls -l"),
		read("ask for both at once", "ls -la"),
		read("ask ls what else it can do", "ls --help"),
		// The word is in the *name* of a dot folder, which is the whole point of
		// the card: plain `ls` will not show it.
		hunt("something new just appeared here", "ls -a"),
	},
	2: {
		read("look before you read", "ls"),
		read("read the note", "cat notes/todo.txt"),
		fix("make a folder named done", "mkdir done"),
		fix("create an empty file named first.txt", "touch first.txt"),
		fix("move first.txt into done/", "mv first.txt done/"),
		fix("copy notes/todo.txt into done/", "cp notes/todo.txt done/"),
		fix("move that copy out of", "mv done/todo.txt backup.txt"),
		// The boundary card: one `pwd`, and the folder the next three cards act in.
		read("see the folder the next three commands act in", "pwd"),
		// The regroup card before the deleting: two listings, then the read tap.
		move{at: "look at everything you have made so far", type_: []string{"ls", "ls done", ""}},
		fix("make a throwaway file named throwaway.txt", "touch throwaway.txt"),
		fix("delete it", "rm throwaway.txt"),
		fix("delete the whole done/ folder", "rm -r done"),
		hunt("the folder holds another", "cat notes/ideas.txt"),
	},
	3: {
		read("make the shell say something", `echo "hello"`),
		fix("save that line into a file called greeting.txt", `echo "hello" > greeting.txt`),
		fix("save both lists", "cat today.txt later.txt > both.txt"),
		// Deliberately not the word the hint suggests: the step takes any line.
		fix("add a line of your own to greeting.txt", `echo "see you" >> greeting.txt`),
		tap("the difference, in one line"),
		read("let the calculator work through", "bc < problems.txt"),
		fix("save those four answers", "bc < problems.txt > answers.txt"),
		read("answer the calculator without writing", `echo "2 + 3" | bc`),
		fix("still read it on screen", `echo "all done" | tee done.txt`),
		hunt("a folder called inbox/", "ls", "cat inbox/delivery.txt"),
	},
	4: {
		read("pipe a file into a counter", "cat today.log | wc -l"),
		read("count something other than lines", "cat today.log | wc -w"),
		fix("save the number of lines in both", "cat logs/2026/*.log | wc -l > lines-2026.txt"),
		read("see the lines that mention 404", "grep 404 today.log"),
		read("ask for the number instead", "grep -c 404 today.log"),
		fix("save the requests from 10.0.0.1", "grep 10.0.0.1 today.log | tee my-visitors.txt"),
		read("put the requested paths in order", "sort urls.txt"),
		read("dropping the repeats", "uniq urls.txt"),
		fix("save the distinct paths", "sort urls.txt | uniq > distinct.txt"),
		read("find every log file in here", `find . -name "*.log"`),
		fix("save the list of log files", `find . -name "*.log" > log-files.txt`),
		read("look for 404s in every file here", "grep -R 404 ."),
		fix("files that mention 404", "grep -R -l 404 . > with-404.txt"),
		hunt("the tree grew while you were working", "ls logs", "cat logs/2024/archive/september.log"),
	},
	5: {
		read("look inside a file you have never made", "cat hello"),
		read("look at its permissions", "ls -l hello"),
		fix("give hello permission to run", "chmod +x hello"),
		read("see the new x's", "ls -l"),
		// The failure before the win: the bare name does not work, and *then* the
		// level says why, and only then does `./hello` land.
		read("try running it by name", "hello"),
		read("see where bash looks for commands", "echo $PATH"),
		fix("run your tool and save what it says", "./hello > out.txt"),
		// The hunt. Reading the script gives nothing away — that is the whole
		// design of it — so the level does not move until it is run.
		hunt("a few new files have appeared",
			"ls",
			"grep -Rl bash .",
			"cat start",
			"chmod +x start",
			"./start",
		),
	},
	6: {
		read("see the permissions of what is here", "ls -l"),
		tap("the mode, spelled out"),
		fix("take your own write permission away", "chmod u-w data.txt"),
		fix("let everyone read public.txt", "chmod a+r public.txt"),
		fix("take the group and everyone else's write away", "chmod go-w notes.txt"),
		tap("the same three letters, in numbers"),
		fix("make data.txt private again", "chmod 600 data.txt"),
		fix("make notes.txt group-readable", "chmod 640 notes.txt"),
		read("compare the two modes you have made", "ls -l"),
		// The vault: seeded 0600, so `ls` lists the folder's contents but
		// `cat` through it is denied. The replay walks that exact path — the
		// denial is the lesson, so it has to be walked, not skipped.
		read("see what is inside secret/", "ls secret"),
		read("try to read the note inside it", "cat secret/flag.txt"),
		read("read the folder's own mode", "ls -ld secret"),
		fix("give secret/ back the x", "chmod 700 secret"),
		read("ask the machine who you are", "id"),
		tap("so what is a group, then?"),
		tap("so what does root get to do"),
		hunt("you gave secret/ the x it was missing", "cd secret", "cat flag.txt"),
	},
}

// TestALevelCanBeFinished types a whole level the way a student would and checks
// the level says so at the end. It is the end-to-end one: content, checks and
// layout together, on a real terminal, with real commands doing real work in a
// throwaway playground.
//
// TUXLAND_REPLAY=N plays just that level; TUXLAND_DUMP=1 prints the screen of
// each one, pass or fail.
func TestALevelCanBeFinished(t *testing.T) {
	needRealSession(t)
	bin := buildGame(t)

	for _, num := range levelsToReplay(t) {
		t.Run(levelName(num), func(t *testing.T) {
			for _, prog := range needs[num] {
				if _, err := exec.LookPath(prog); err != nil {
					t.Skipf("level %d types %s, which this machine does not have", num, prog)
				}
			}
			script, ok := replays[num]
			if !ok {
				t.Fatalf("level %d has no replay script", num)
			}
			s := playGame(t, bin, widthsToPlay(t)[0], []string{"-level", strconv.Itoa(num)}, t.TempDir())
			t.Cleanup(func() {
				if t.Failed() || os.Getenv("TUXLAND_DUMP") != "" {
					s.show()
				}
			})
			// Where this level began, so the closing check can look at the whole
			// run: the wait after the last command may already have consumed the
			// banner as one of its two legitimate endings.
			from := s.mark()

			for i, m := range script {
				s.expectCard(m.at, i)
				for _, cmd := range m.type_ {
					// Everything from this mark is this line's doing, so the wait
					// after it cannot be satisfied by a prompt that was already on
					// screen — which is the race that makes a naive replay type the
					// next line into a shell that is still busy with this one.
					at := s.mark()
					s.enter(cmd)
					s.expectEcho(cmd)
					s.expectAnswer(at, num)
				}
			}
			s.expectLevelComplete(from, num)
		})
	}
}

// TestEveryStepHasAMoveInItsOwnReplay keeps the table honest against the content,
// which the replay itself cannot do. A replay waits for a card by looking at the
// whole screen, so a card left on screen by an earlier step can answer for a later
// one: drop a step into the middle of a level and the moves after it still match,
// the level stops one card early, and TestALevelCanBeFinished fails with nothing
// pointing at the step that moved. Checking here instead makes the failure say what
// it is — one move per step, in order, each naming its own step's goal.
func TestEveryStepHasAMoveInItsOwnReplay(t *testing.T) {
	stepsOf := map[int][]*step{}
	eachStep(func(lv *level, st *step) {
		stepsOf[lv.num] = append(stepsOf[lv.num], st)
	})
	for _, lv := range levels {
		// The toolbox is not replayed: it is reached with ↓ from the menu rather
		// than -level, and two of its cards want a key rather than a line, which
		// is what playExtra in pty_test.go is for.
		if lv.isExtra() {
			continue
		}
		steps := stepsOf[lv.num]
		script, ok := replays[lv.num]
		if !ok {
			t.Errorf("level %d (%s) has no replay script", lv.num, lv.name)
			continue
		}
		if len(script) != len(steps) {
			t.Errorf("level %d (%s): the replay types %d moves for %d steps", lv.num, lv.name, len(script), len(steps))
			continue
		}
		for i, m := range script {
			if !strings.Contains(asShown(steps[i].goal), asShown(m.at)) {
				t.Errorf("level %d (%s), step %d: the replay waits for %q, but the card says %q",
					lv.num, lv.name, i+1, m.at, steps[i].goal)
			}
		}
	}
}

// levelsToReplay is every numbered level, or the one TUXLAND_REPLAY names.
func levelsToReplay(t *testing.T) []int {
	t.Helper()
	spec := os.Getenv("TUXLAND_REPLAY")
	if spec == "" {
		return levelNumbers()
	}
	n, err := strconv.Atoi(strings.TrimSpace(spec))
	if err != nil {
		t.Fatalf("TUXLAND_REPLAY=%q: not a level number", spec)
	}
	if _, ok := replays[n]; !ok {
		t.Fatalf("TUXLAND_REPLAY=%d: no replay script for that level", n)
	}
	return []int{n}
}

func levelNumbers() []int {
	out := make([]int, 0, NumLevels())
	for n := 1; n <= NumLevels(); n++ {
		out = append(out, n)
	}
	return out
}

func levelName(n int) string {
	for _, lv := range levels {
		if lv.num == n {
			return fmt.Sprintf("level %d %s", n, lv.name)
		}
	}
	return fmt.Sprintf("level %d", n)
}
