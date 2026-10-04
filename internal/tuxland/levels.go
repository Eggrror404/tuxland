package tuxland

import (
	"os"
)

// kind is what a step asks for: look (info), do (task) or find (flag).
type kind int

const (
	kindInfo kind = iota
	kindTask
	kindFlag
)

// level is one chapter of tuxland: a few guided sections, ending in a flag
// hunt. The numbered levels walk the command line in order, and its one
// *Input & output* family is two levels here — the constructs first,
// then the tools that run on them — because searching needs the plumbing and the
// plumbing is worth its own level. The last entry is the habits toolbox:
// unnumbered, menu-only, and never part of the sequence.
type level struct {
	num      int
	extra    bool     // the habits toolbox: no number, not in the sequence, menu-only
	name     string   // folder + --list: navigation
	emoji    string   // banner
	title    string   // banner: WHERE AM I?
	tagline  string   // --list / kickoff: where you stand
	lead     string   // banner's one-liner
	cmds     []string // --list / kickoff: the command family
	used     []string // level-complete summary
	needs    []string // programs this level types at: without one it cannot be played
	seed     []plant  // the playground, scaffolded fresh every run
	sections []section
}

// section groups a handful of steps so a level never feels crowded. The lead is
// one line of orientation under the header — what this stretch is about.
type section struct {
	title string
	lead  string
	steps []step
}

// step is one beat. Backticks in goal/note/hints mark real commands and file
// names — the game renders those spans in its own voice.
type step struct {
	kind    kind
	goal    string
	note    string
	hints   []string
	done    string  // the ✅ / 🎉 line
	noCmd   bool    // info step with nothing to run: one Enter moves on
	exitKey bool    // the beat is about Ctrl-D: the game reads that key itself
	plant   []plant // placed the moment the step becomes active
	check   check   // kindTask: the artifact we look for
	token   string  // kindFlag: the word we watch the screen for
	// assembled marks a hunt whose token is in no file: the script builds it
	// when it runs, so reading the file must *not* be enough. It inverts the
	// usual invariant — the token has to be absent from the tree, not in it.
	assembled bool
}

// plant is one thing the game puts in the playground — a file or a folder, at a
// playground-relative path, with an exact mode. A level's seed is planted when
// the level is scaffolded; a step's plants go in when the step becomes active,
// so a flag hunt's riddle appears when and only when the hunt starts.
type plant struct {
	at   string      // playground-relative: "note.txt", "secret/flag.txt"
	dir  bool        // make a directory instead (its text is ignored)
	mode os.FileMode // exact mode; 0 = default (0o644 file, 0o755 directory)
	text string      // file content
}

// put plants p into dir, creating the folder chain above it.
func (p plant) put(dir string) error {
	if p.dir {
		return putDir(dir, p.at, p.modeOr(0o755))
	}
	return putMode(dir, p.at, p.modeOr(0o644), p.text)
}

// modeOr is the explicit-or-default mode: leave the mode alone unless a step is
// graded on it, and the umask never decides a graded mode anyway (putMode and
// putDir chmod).
func (p plant) modeOr(def os.FileMode) os.FileMode {
	if p.mode != 0 {
		return p.mode
	}
	return def
}

// plantAll puts every plant into dir in order, stopping at the first miss.
func plantAll(dir string, ps []plant) error {
	for _, p := range ps {
		if err := p.put(dir); err != nil {
			return err
		}
	}
	return nil
}

// levels is every entry the menu can show, in menu order. The extra is last;
// numberedLevels() is the sequence and extraLevel() is the toolbox.
var levels = []*level{level1, level2, level3, level4, level5, level6, habitsLevel}

// isLast is the end of the numbered sequence: finishing it rolls straight into
// the finale. The extra is never last — finishing it returns to the menu.
func (lv *level) isLast() bool { return lv.num == NumLevels() }

func (lv *level) isExtra() bool { return lv.extra }

// numberedLevels is the sequence levels. The extra sits last in levels so
// the menu lists it last, but it is not part of the run: it is never "the next
// level", the digit keys stop at NumLevels(), and -level cannot reach it.
func numberedLevels() []*level { return levels[:len(levels)-1] }

func extraLevel() *level { return levels[len(levels)-1] }

// ── Level 1 · navigation · pwd cd ls ─────────────────────────────────────────

var level1 = &level{
	num: 1, name: "navigation", emoji: "🧭",
	title: "WHERE AM I?", tagline: "where you stand",
	lead: "every command happens somewhere — a real folder.",
	cmds: []string{"pwd", "cd", "ls", "ls -a", "ls -l", "ls -la"},
	used: []string{"pwd", "cd", "ls", "ls -a", "ls -l", "ls -la", "ls --help"},
	// A couple of folders to walk between, a dot-file to explain `ls -a`, and
	// nothing else: level 1 is pure navigation. The flag folder is planted when
	// the hunt starts, so plain `ls` never shows it early.
	seed: []plant{
		{at: "readme.txt", text: "welcome to the lab playground.\nlook around — that is the whole first level.\n"},
		{at: "notes/readme.txt", text: "a folder inside a folder: `cd notes` walks in.\n"},
		{at: "photos", dir: true, mode: 0o755},
		{at: ".hidden.txt", text: "a name that starts with a dot stays invisible to plain `ls`.\n"},
	},
	sections: []section{
		{
			title: "where are you",
			lead:  "before anything else: where does a command land?",
			steps: []step{
				{
					kind: kindInfo,
					goal: "ask where you are",
					note: "run `pwd` — print working directory. read it, then press Enter.",
					hints: []string{
						"`pwd`",
						"it prints the full path of this folder",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "step into `notes/` and ask again",
					note: "`cd notes` walks in, then `pwd` shows the change. read it, then press Enter.",
					hints: []string{
						"`cd notes`",
						"then `pwd` — the path ends in `notes` now",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "step back out",
					note: "`cd ..` goes up one folder. read it, then press Enter.",
					hints: []string{
						"`cd ..`",
						"then `pwd` — you are back where you started",
					},
					done: "seen it — moving on",
				},
			},
		},
		{
			title: "what's here",
			lead:  "`ls` answers the question `pwd` only half-answers.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "see what's in this folder",
					note: "`ls` — list. read it, then press Enter.",
					hints: []string{
						"`ls`",
						"every name you see is a real file or folder",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "see the hidden names too",
					note: "`ls -a` — the `-a` shows everything, dot-files included. read it, then press Enter.",
					hints: []string{
						"`ls -a`",
						"a name starting with `.` is invisible to plain `ls`",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "see sizes, dates and permissions",
					note: "`ls -l` — the long listing. read it, then press Enter.",
					hints: []string{
						"`ls -l`",
						"each line is one entry: mode, owner, size, date, name",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "ask for both at once",
					note: "`ls -la` — short flags stack: one dash, then both letters. read it, then press Enter.",
					hints: []string{
						"`ls -la`",
						"the same two flags you just used, in one command — long ones spell it out instead",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "ask `ls` what else it can do",
					// Do not send a beginner hunting through this. `ls --help` is a
					// page of dense noise, much of it OSC-8 hyperlink escapes,
					// which a terminal without hyperlink support prints inline —
					// so the first flag they need can arrive as
					// `-a-a, --allo`. The lesson is that the manual exists and that
					// nobody reads one cold, not finding a flag in it.
					note: "`ls --help` — most commands can print their own manual, and it is a page of " +
						"dense noise. that is normal and you are not meant to read it now: the point is that " +
						"it is *there* when you need one flag. read it, then press Enter.",
					hints: []string{
						"`ls --help`",
						"scroll away — knowing it exists is the whole lesson here",
					},
					done: "seen it — moving on",
				},
			},
		},
		{
			title: "find the flag",
			lead:  "the last beat of the level: a magic word, hidden somewhere here.",
			steps: []step{
				{
					kind: kindFlag,
					goal: "something new just appeared here — and its *name* is the magic word",
					note: "the listing you took a moment ago is already out of date: one of the names here starts " +
						"with a dot, and that one is new. the word looks like `linuxlab-…` — I'll spot it the " +
						"moment it appears on your screen. nothing to type back to me.",
					hints: []string{
						"the short listing never shows a name that starts with a dot",
						"ask for the hidden ones the way this level taught you, then read the names word by word",
						"`ls -a`",
					},
					// The folder exists only from here on, so `ls -a` before the
					// hunt shows a dot-file but never the flag.
					plant: []plant{
						{at: ".linuxlab-h1dd3n", dir: true, mode: 0o755},
					},
					done:  "that folder's name was the word.",
					token: "linuxlab-h1dd3n",
				},
			},
		},
	},
}

// ── Level 2 · files · cat mkdir touch mv cp rm ───────────────────────────────

var level2 = &level{
	num: 2, name: "files", emoji: "📁",
	title: "YOUR FIRST FILES", tagline: "make, keep, throw away",
	lead: "files are the whole point of a computer.",
	cmds: []string{"cat", "mkdir", "touch", "mv", "cp", "rm"},
	used: []string{"cat", "mkdir", "touch", "mv", "cp", "rm", "rm -r"},
	seed: []plant{
		{at: "notes/todo.txt", text: "learn the moves:\n  cat  mkdir  touch  mv  cp  rm\n"},
		{at: "notes/ideas.txt", text: "a log full of clues would make a good next level\nthe magic word is linuxlab-n0t3m4n\n"},
	},
	sections: []section{
		{
			title: "read first",
			lead:  "before you make anything, read what is already here.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "look before you read",
					note: "`ls` first — always look before you open something. read the output, then press Enter.",
					hints: []string{
						"`ls`",
						"one of the names is a folder: `notes/`",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "read the note",
					note: "`cat notes/todo.txt` prints a file's contents. read it, then press Enter.",
					hints: []string{
						"`cat notes/todo.txt`",
						"the list is the moves this level teaches",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "make a folder named `done`",
					note: "`mkdir` = make directory",
					hints: []string{
						"`mkdir done`",
						// Used to be "`ls` shows it next to `notes/`", which is true and
						// useless: a plain `ls` prints the bare name `done` in among the
						// files, with nothing on the line saying it is a *folder*. A
						// student who ran it had done the task and had no way to know,
						// which is the one thing a hint must never do. `ls -l` puts a
						// `d` in the first column and answers it.
						"`ls -l` — the `d` in the first column is the folder",
					},
					done:  "`done/` is there.",
					check: isDir("done"),
				},
				{
					kind: kindTask,
					goal: "create an empty file named `first.txt`",
					note: "`touch` makes a file appear",
					hints: []string{
						"`touch first.txt`",
						"`ls` to see it",
					},
					done:  "`first.txt` is born.",
					check: isFile("first.txt"),
				},
				{
					kind: kindTask,
					goal: "move `first.txt` into `done/`",
					note: "`mv` moves a file — gone from here, there instead.",
					hints: []string{
						"`mv first.txt done/`",
						"`ls` — gone from here · `ls done` — it lives there now",
					},
					done:  "moved — `first.txt` now lives in `done/`.",
					check: all(isGone("first.txt"), isFile("done/first.txt")),
				},
			},
		},
		{
			title: "keep a copy",
			lead:  "`cp` leaves the original alone; `mv` does not.",
			steps: []step{
				{
					kind: kindTask,
					goal: "copy `notes/todo.txt` into `done/`",
					note: "after a copy, *both* files must exist",
					hints: []string{
						"`cp notes/todo.txt done/`",
						"`ls done` — the copy is in there, and `notes/todo.txt` never moved",
					},
					done:  "two copies now — the original never moved.",
					check: all(isFile("notes/todo.txt"), isFile("done/todo.txt")),
				},
				{
					kind: kindTask,
					goal: "move that copy out of `done/` and rename it `backup.txt`",
					note: "`mv` moves *and* renames in one command",
					hints: []string{
						"`mv done/todo.txt backup.txt`",
						"afterwards `ls done` holds only `first.txt`",
					},
					done:  "`backup.txt` — moved and renamed in one go.",
					check: all(isGone("done/todo.txt"), isFile("backup.txt")),
				},
			},
		},
		{
			title: "no trash can",
			lead:  "the one command here that gives nothing back — so look first.",
			steps: []step{
				{
					// Eight graded cards in a row was where this level lost people,
					// and this is the natural seam: everything created so far is
					// visible, and the next three cards delete. The habit this section
					// is about is *looking*, so looking is the beat.
					kind: kindInfo,
					goal: "look at everything you have made so far",
					note: "`ls`, then `ls done` — the top of the folder, and the inside of the one you made. both listings are worth a habit: the next three cards make a file, then throw it away, then throw away a whole folder, and two of those three give nothing back. knowing what is there is the only way to know what you lose. read them, then press Enter.",
					hints: []string{
						"`ls`, then `ls done`",
						"`done/` still holds `first.txt` — `todo.txt` went out to `backup.txt`",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "make a throwaway file named `throwaway.txt`",
					hints: []string{
						"`touch throwaway.txt`",
						"then `ls` to see it",
					},
					done:  "`throwaway.txt` is there.",
					check: isFile("throwaway.txt"),
				},
				{
					kind: kindTask,
					goal: "delete it — for good",
					note: "`rm` has no trash can: look before you `rm`",
					hints: []string{
						"`rm throwaway.txt`",
						"gone for good — `ls` does not list it any more",
					},
					done:  "gone. no trash can, no undo — always look first.",
					check: isGone("throwaway.txt"),
				},
				{
					kind: kindTask,
					goal: "delete the whole `done/` folder in one command",
					note: "a folder needs `-r` — plain `rm` only takes files, and bash will say so",
					hints: []string{
						"try plain `rm done` first if you like: the error tells you which flag it wants",
						"`rm -r done`",
						"`-r` is short for `--recursive`: the folder *and* everything inside it",
					},
					done:  "`done/` is gone — contents and all.",
					check: isGone("done"),
				},
			},
		},
		{
			title: "find the flag",
			lead:  "the last beat of the level: a magic word, hidden somewhere here.",
			steps: []step{
				{
					kind: kindFlag,
					goal: "you read one note in `notes/` — the folder holds another, and *it* remembers the word",
					note: "the word is not in a name here: it is *inside* a file, so it has to be printed out before " +
						"I can see it. it looks like `linuxlab-…` — I'll spot it the moment it appears on your " +
						"screen. nothing to type back to me.",
					hints: []string{
						"two files live in `notes/` — one of them is only the list of commands",
						"the word sits at the end of the second line of the other file",
						"`cat notes/ideas.txt` — or `cat notes/*.txt`, which reads both at once",
					},
					done:  "the notes remember things.",
					token: "linuxlab-n0t3m4n",
				},
			},
		},
	},
}

// ── Level 3 · text in, text out · echo > >> < | tee ────────────────────────────
//
// The ways text moves. Two of the four constructs are only demonstrable with
// something on the receiving end, so this level borrows exactly one tool — `bc`,
// the calculator — for `<` and `|`, and gets nothing else out of it. The level
// stays flat with no folders: searching a tree needs a tree, and that is level
// 4's whole subject.
//
// `bc` earns the borrow because it is a *program*, and a program reads its input
// rather than opening a file, which is the only honest reason to reach for `<`:
// the same shape a programming problem has when the judge hands you the input as
// a file. GNU `bc` does also accept a file name, so `bc problems.txt` prints the
// same four answers — this level therefore does not claim `<` *changes* the
// output, only that it is how you hand a program its work instead of typing it.
// (`tr` would prove the stronger point — it has no file operand at all — but that
// is a second tool to carry, and `bc` is the one a beginner can read.)
//
// The two to-do lists have different lengths on purpose — short and short — so §1's
// second task can grade their sum without the student having to add. `problems.txt`
// is a handful of expressions, and `bc` answers each one on its own line.

const todayList = `buy milk
send the homework
water the plants
`

const laterList = `book the room
charge the laptop
`

const problems = `2 + 3
12 * 12
100 / 4
7 - 9
`

var level3 = &level{
	num: 3, name: "text-io", emoji: "🔀",
	title: "TEXT IN, TEXT OUT", tagline: "where the output goes",
	lead: "a command does not just print — it hands its output on. here is every way to hand it somewhere.",
	cmds: []string{"echo", ">", ">>", "<", "|", "tee", "bc"},
	used: []string{"echo", "cat", ">", ">>", "<", "|", "tee", "bc"},
	// The game's one program beyond a base system: `bc < problems.txt` and
	// `echo "2 + 3" | bc` are the cards that teach `<` and `|`, and there is no
	// rewording that teaches them without a calculator. So it is checked, not
	// hoped for — see game.noTools.
	needs: []string{"bc"},
	seed: []plant{
		{at: "today.txt", text: todayList},
		{at: "later.txt", text: laterList},
		{at: "problems.txt", text: problems},
		{at: "about.txt", text: `three plain files live here:

  today.txt      what is on the list today — 3 lines
  later.txt      what can wait — 2 lines
  problems.txt   a sheet of arithmetic — 4 sums, one per line

nothing here is precious. break it, delete it, make more.
`},
	},
	sections: []section{
		{
			title: "say a line",
			lead:  "`echo` makes a line of text. on its own it just prints it and throws it away.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "make the shell say something",
					note: "`echo \"hello\"` — `echo` prints whatever you give it and adds a newline at the end. " +
						"the quotes are for the shell, not for `echo`: they keep the spaces together. read it, then press Enter.",
					hints: []string{
						"`echo \"hello\"`",
						"nothing has to exist yet — `echo` only makes text",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "save that line into a file called `greeting.txt`",
					note: "`>` takes whatever a command printed and puts it in a file instead of on your screen",
					hints: []string{
						"the same `echo`, with `>` and a file name at the end",
						"`echo \"hello\" > greeting.txt`",
						"`cat greeting.txt` — it went to the file, not to the screen",
					},
					done:  "saved — `cat greeting.txt` shows the line.",
					check: all(isFile("greeting.txt"), contentContains("greeting.txt", "hello")),
				},
				{
					kind: kindTask,
					goal: "save *both* lists — today's and the later one — into a file called `both.txt`",
					note: "`>` catches whatever a command printed, so it works for `cat` too — and `cat` takes as " +
						"many names as you like, which is why two files still fit on one line",
					hints: []string{
						"`cat` first this time, then `>` and the file name",
						"`cat today.txt later.txt > both.txt`",
						"`cat both.txt` — three lines, then two",
					},
					done:  "saved — three lines and two, in one file.",
					check: all(isFile("both.txt"), lineCountIs("both.txt", 5)),
				},
			},
		},
		{
			title: "add to it, don't wipe it",
			lead:  "`>` empties a file before it writes into it. `>>` does not.",
			steps: []step{
				{
					kind: kindTask,
					goal: "add a line to `greeting.txt` without losing the one already in there",
					note: "`>>` appends — a second `>` would have wiped the file first",
					hints: []string{
						"`echo \"goodbye\" >> greeting.txt`",
						"`cat greeting.txt` — two lines now, the first one still there",
						"if only your line is in there you used one `>`: save it again, then append",
					},
					done:  "appended — both lines are in there.",
					check: all(contentContains("greeting.txt", "goodbye"), contentContains("greeting.txt", "hello")),
				},
				{
					kind:  kindInfo,
					noCmd: true,
					goal:  "the difference, in one line",
					note:  "nothing to run this time: `>` *replaces* what was in the file, `>>` *adds* to what was there. press Enter when you've read it.",
					hints: []string{"nothing to run — press Enter"},
					done:  "seen it — moving on",
				},
			},
		},
		{
			title: "hand a file in",
			lead: "so far every command has said which file it wants. a program does not have to — it reads " +
				"what arrives on its input, the way a calculator does.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "let the calculator work through the sheet instead of you",
					note: "`bc < problems.txt` — four answers, one per line, and you did not type a single sum. " +
						"`bc` reads input rather than opening a file, so the sheet has to arrive on the *input* side. " +
						"this is the shape every program in a programming problem has: the test data comes to you as " +
						"a file. read the answers, then press Enter.",
					hints: []string{
						"`bc < problems.txt`",
						"the answers are 5, 144, 25 and -2 — all four, one per line",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "save those four answers into a file called `answers.txt`",
					note: "the same command with `>` and a file name at the end — in from the left, out to the right",
					hints: []string{
						"the same command again, plus `> answers.txt` at the end",
						"`bc < problems.txt > answers.txt`",
						"`cat answers.txt` — four lines, the answers in order",
					},
					done:  "saved — input from one side, output to the other, one command.",
					check: all(isFile("answers.txt"), lineCountIs("answers.txt", 4), contentContains("answers.txt", "144")),
				},
			},
		},
		{
			title: "see it *and* save it",
			lead:  "`|` hands one command's output straight to the next one. `tee` is what sits in the middle of that and does both jobs.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "answer the calculator without writing a file at all",
					note: "`echo \"2 + 3\" | bc` — the pipe hands `bc` the output of `echo`, so the calculator got its " +
						"input from a command instead of a file. read the answer, then press Enter.",
					hints: []string{
						"`echo \"2 + 3\" | bc`",
						"the pipe goes between the two commands, with a space on each side — and `bc` was given no file name at all",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "write the line `all done` into `done.txt` — and still read it on screen",
					note: "`tee` writes the file *and* lets the lines through, so they land in the file and on your " +
						"screen at once. this is the one place where the difference is something you can see.",
					hints: []string{
						"the middle command of the pipeline is the one doing both",
						"`tee` is the middle: it writes the file and lets the lines through",
						"`echo \"all done\" | tee done.txt`",
					},
					done:  "saved *and* seen — `tee` did both at once.",
					check: all(isFile("done.txt"), contentContains("done.txt", "all done")),
				},
			},
		},
		{
			title: "find the flag",
			lead:  "the last beat of the level: a magic word, hidden somewhere here.",
			steps: []step{
				{
					kind: kindFlag,
					goal: "a folder called `inbox/` has appeared — and nothing in it was there a moment ago",
					note: "the word is not in a name this time: it is *inside* one of the two files that landed, so it " +
						"has to be printed out before I can see it. it looks like `linuxlab-…` — I'll spot it the " +
						"moment it appears on your screen. nothing to type back to me.",
					hints: []string{
						"two files came in together, and one of them is only a list of what arrived",
						"the word sits in the other one, on a line of its own",
						"`ls inbox`, then `cat inbox/delivery.txt`",
					},
					// The folder and its two files exist only from here on, so
					// nothing the student can reach earlier holds the word.
					plant: []plant{
						{at: "inbox", dir: true, mode: 0o755},
						{at: "inbox/manifest.txt", text: "delivery 2026-10-06\ntwo items arrived this morning\n"},
						{at: "inbox/delivery.txt", text: "a stowaway rode in with the parcel:\nlinuxlab-r3d1r\n"},
					},
					done:  "the delivery note remembers.",
					token: "linuxlab-r3d1r",
				},
			},
		},
	},
}

// ── Level 4 · the tools you pipe into · cat wc grep find sort uniq ─────────────
//
// Level 3 moved text around and could do nothing with it. This is the other
// half: the commands that *read* text, which is what makes the pipe worth
// having. They are all one idea — take text in, hand back a smaller answer —
// and they only earn their keep at the end of a pipeline, which is why every
// step here has a `|` in it somewhere.
//
// `cat` opens the level as a revisit on purpose: it is the one tool the student
// has met twice already, and seeing it here with a job to do makes the shape of
// the level obvious before the unfamiliar ones arrive.
//
// The tree is the point of the second half: `find` and `grep -R` have nothing
// to do on one file. `today.log` has exactly 10 lines — 2 ERROR, 2 ×404,
// 4 ×10.0.0.1 — and `urls.txt` is 8 lines with 5 distinct paths in a jumbled
// order, none of the repeats next to each other, so `sort` then `uniq` is the
// only way to the answer.

const todayLog = `10.0.0.1 - - [06/Sep/2026:09:00:01] "GET / HTTP/1.1" 200 512
10.0.0.1 - - [06/Sep/2026:09:00:04] "GET /style.css HTTP/1.1" 200 128
10.0.0.7 - - [06/Sep/2026:09:00:09] "GET /api/notes HTTP/1.1" 200 2048
192.168.0.5 - - [06/Sep/2026:09:00:14] "GET /missing.html HTTP/1.1" 404 0
10.0.0.1 - - [06/Sep/2026:09:00:19] "GET /images/logo.png HTTP/1.1" 200 4096
10.0.0.2 - - [06/Sep/2026:09:00:23] "GET /api/x HTTP/1.1" 401 0
192.168.0.7 - - [06/Sep/2026:09:00:28] "GET /admin HTTP/1.1" 500 0 ERROR: no such user
10.0.0.2 - - [06/Sep/2026:09:00:33] "GET /old HTTP/1.1" 404 0
10.0.0.7 - - [06/Sep/2026:09:00:37] "GET /health HTTP/1.1" 500 0 ERROR: upstream is grumpy
10.0.0.1 - - [06/Sep/2026:09:00:42] "GET /favicon.ico HTTP/1.1" 200 0
`

const urls = `/style.css
/index.html
/style.css
/images/logo.png
/index.html
/api/notes
/favicon.ico
/index.html
`

var level4 = &level{
	num: 4, name: "read-it", emoji: "🔍",
	title: "READING WITH PIPES", tagline: "the tools at the end of a pipe",
	lead: "level 3 moved text around. these are the commands that read it — and they only earn their keep at the end of a pipe.",
	cmds: []string{"cat", "wc -l", "wc -w", "grep", "grep -c", "grep -l", "grep -R", "find", "sort", "uniq"},
	used: []string{"cat", "wc -l", "wc -w", "grep", "grep -c", "grep -l", "grep -R", "find", "sort", "uniq", "|", ">"},
	seed: []plant{
		{at: "today.log", text: todayLog},
		{at: "urls.txt", text: urls},
		{at: "about.txt", text: `today's traffic is today.log, right here in this folder.
urls.txt is the list of paths people asked for, in the order they asked.

the older logs live under logs/, one folder per year:
  logs/2026/   this year
  logs/2025/   last year — and archive/ for what got moved out of the way
`},
		{at: "logs/2026/january.log", text: `10.0.0.1 - - [06/Jan/2026:10:00:01] "GET / HTTP/1.1" 200 512
10.0.0.2 - - [06/Jan/2026:10:14:31] "GET /style.css HTTP/1.1" 200 128
10.0.0.7 - - [06/Jan/2026:10:22:09] "GET /api/notes HTTP/1.1" 200 2048
`},
		{at: "logs/2026/february.log", text: `10.0.0.1 - - [06/Feb/2026:09:00:02] "GET / HTTP/1.1" 200 498
10.0.0.7 - - [06/Feb/2026:09:00:55] "GET /api/x HTTP/1.1" 401 0
`},
		{at: "logs/2025/december.log", text: `10.0.0.2 - - [06/Dec/2025:23:40:10] "GET /missing.html HTTP/1.1" 404 0
10.0.0.1 - - [06/Dec/2025:23:44:02] "GET / HTTP/1.1" 200 512
`},
		{at: "logs/2025/archive/november.log", text: `10.0.0.2 - - [06/Nov/2025:08:11:20] "GET /a HTTP/1.1" 404 0
10.0.0.7 - - [06/Nov/2025:08:15:44] "GET /favicon.ico HTTP/1.1" 404 0
`},
	},
	sections: []section{
		{
			title: "count first",
			lead:  "`cat` hands its whole input on. `wc` hands back only the number you asked for. that is the whole trade.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "pipe a file into a counter",
					note: "`cat today.log | wc -l` — `cat` prints all ten lines and `wc -l` counts them. " +
						"the pipe is where the text changed hands. read it, then press Enter.",
					hints: []string{
						"`cat today.log | wc -l`",
						"nothing was written to a file — the answer went to your screen",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "count something other than lines",
					note: "`cat today.log | wc -w` — same command, different letter: words this time. " +
						"the letters are `-l` lines, `-w` words, `-c` characters. read it, then press Enter.",
					hints: []string{
						"`cat today.log | wc -w`",
						"`wc` with no letter at all gives all three at once",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "save the number of lines in both of this year's logs into `lines-2026.txt`",
					note: "`>` is old news by now — it still does exactly this at the end of a pipe",
					hints: []string{
						"`cat` both files, pipe into `wc -l`, then `>`",
						"`cat logs/2026/january.log logs/2026/february.log | wc -l > lines-2026.txt`",
						"`cat lines-2026.txt` — three and two, added up",
					},
					done:  "saved — five lines across the two files.",
					check: all(isFile("lines-2026.txt"), contentContains("lines-2026.txt", "5")),
				},
			},
		},
		{
			title: "find the lines that matter",
			lead:  "`grep` keeps the lines that match something and throws the rest away.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "see the lines that mention 404",
					note: "`grep 404 today.log` — `grep` prints the lines that match, whole, in order. the pattern " +
						"is plain text, and it matches anywhere in a line. read it, then press Enter.",
					hints: []string{
						"`grep 404 today.log`",
						"two of the ten lines mention it",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "ask for the number instead of the lines",
					note: "`grep -c 404 today.log` — `-c` counts the matches rather than printing them, so nothing " +
						"scrolls past. read it, then press Enter.",
					hints: []string{
						"`grep -c 404 today.log`",
						"one letter, and the whole output shrinks to a single number",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "save the requests from `10.0.0.1` into `my-visitors.txt`",
					note: "a pipeline that keeps whole lines — `grep` picks them, `tee` saves them and you still see them",
					hints: []string{
						"`grep 10.0.0.1 today.log | tee my-visitors.txt`",
						"four lines start with that address",
					},
					done:  "saved — your own little corner of the log.",
					check: all(isFile("my-visitors.txt"), lineCountIs("my-visitors.txt", 4)),
				},
			},
		},
		{
			title: "sort, then uniq",
			lead:  "these two only make sense together, and the order matters.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "put the requested paths in order",
					note: "`sort urls.txt` — `sort` rearranges its input into order and prints it. " +
						"read it, then press Enter.",
					hints: []string{
						"`sort urls.txt`",
						"eight lines in, eight lines out — same lines, different order",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "now try dropping the repeats — on their own",
					note: "`uniq urls.txt` — `uniq` prints one copy of each run of identical lines. " +
						"look closely: hardly anything disappeared, because the repeats are not next to each other. " +
						"read it, then press Enter.",
					hints: []string{
						"`uniq urls.txt`",
						"this is the part that surprises everyone — `uniq` only compares a line with the one above it",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "save the distinct paths into `distinct.txt` — the five that were actually asked for",
					note: "one pipe, and the order is what makes it work: `sort` puts the copies next to each other, " +
						"then `uniq` can collapse them",
					hints: []string{
						"`uniq` on its own cannot do this — it needs its input sorted first",
						"`sort urls.txt | uniq > distinct.txt`",
						"`cat distinct.txt` — five lines",
					},
					done:  "saved — eight paths in, five distinct ones out.",
					check: all(isFile("distinct.txt"), lineCountIs("distinct.txt", 5)),
				},
			},
		},
		{
			title: "every file, not just this one",
			lead:  "one file is easy. this is where the tools start earning their keep.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "find every log file in here",
					note: "`find . -name \"*.log\"` — `find` starts at `.` (this folder) and looks *inside* " +
						"sub-folders too; `-name \"*.log\"` keeps only the names ending in `.log`. the quotes stop " +
						"the shell from expanding `*` itself. read the list, then press Enter.",
					hints: []string{
						"`find . -name \"*.log\"`",
						"five of them — `today.log` right here, and four under `logs/`, the oldest a folder deeper than the rest",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "save the list of log files into `log-files.txt`",
					note: "`find` prints its list; `>` catches it",
					hints: []string{
						"the same `find`, with `>` and a file name at the end",
						"`find . -name \"*.log\" > log-files.txt`",
						"`cat log-files.txt` — five paths, one per line",
					},
					done:  "saved — five paths in `log-files.txt`.",
					check: all(isFile("log-files.txt"), contentContains("log-files.txt", "today.log"), contentContains("log-files.txt", "january"), contentContains("log-files.txt", "november")),
				},
				{
					kind: kindInfo,
					goal: "look for 404s in *every* file here",
					note: "`grep -R 404 .` — `-R` recurses into sub-folders, and each line comes out prefixed " +
						"with the file it came from. read it, then press Enter.",
					hints: []string{
						"`grep -R 404 .`",
						"the file name in front of each line is the new part — plain `grep` only ever looked at the one file you named",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "save the list of files that mention `404` into `with-404.txt`",
					note: "`-l` prints just the file names, one per line — that is the output you want when you " +
						"are making a list, and a list is what a pipe is for",
					hints: []string{
						"`grep -R` again, this time with `-l`, then `>`",
						"`grep -R -l 404 . > with-404.txt`",
						"`cat with-404.txt` — three files had a 404 in them",
					},
					done:  "saved — three files listed.",
					check: all(isFile("with-404.txt"), contentContains("with-404.txt", "today.log"), contentContains("with-404.txt", "december"), contentContains("with-404.txt", "november")),
				},
			},
		},
		{
			title: "find the flag",
			lead:  "the last beat of the level: a magic word, hidden somewhere here.",
			steps: []step{
				{
					kind: kindFlag,
					goal: "the tree grew while you were working — there is a year under `logs/` that was not there when you listed it",
					note: "two more logs came in, and one of them is filed deeper than the other. the word is *inside* " +
						"one of them, so it has to be printed out before I can see it. it looks like `linuxlab-…` — " +
						"I'll spot it the moment it appears on your screen. nothing to type back to me.",
					hints: []string{
						"two files came in together, and the deeper one is the one that got left open",
						"the word sits inside the deeper log, on a line of its own",
						"`find . -name \"*.log\"` again and read the one under `archive/`",
					},
					// A year that was not in the tree when the student listed it:
					// nothing they already found can reach the word.
					// L4 spends a whole level teaching "the folder is the year" and makes the
					// student practise it, so the years in here have to match the folders. They
					// did not at first: 2025-dated lines sat under logs/2024/, which would
					// have taught the opposite lesson on the one tree that carries the flag.
					plant: []plant{
						{at: "logs/2024", dir: true, mode: 0o755},
						{at: "logs/2024/june.log", text: `10.0.0.4 - - [06/Jun/2024:09:14:02] "GET / HTTP/1.1" 200 501
10.0.0.4 - - [06/Jun/2024:09:14:06] "GET /about HTTP/1.1" 200 220
`},
						{at: "logs/2024/archive", dir: true, mode: 0o755},
						{at: "logs/2024/archive/september.log", text: `10.0.0.3 - - [06/Sep/2024:11:02:07] "GET / HTTP/1.1" 200 498
10.0.0.3 - - [06/Sep/2024:11:02:11] "GET /style.css HTTP/1.1" 200 128
a requester left a stowaway in here: linuxlab-s33rch
`},
					},
					done:  "a corner of the tree you had never walked.",
					token: "linuxlab-s33rch",
				},
			},
		},
	},
}

// ── Level 5 · executables · chmod +x ./ PATH ──────────────────────────────────
//
// The script is *given*, not written. A level that opens "write a one-line
// script" asks for the thing it never taught — what a script file even is — so
// this one starts with a file that already holds a line, and the first job is
// to look inside it. `cat` then `chmod +x` then `./` is the whole story of
// "every command is just a file", and the student is shown each step of it.
//
// The hunt is the same lesson one level on. It used to ask the student to make
// their own script say the word in `message.txt`, which nobody would ever do —
// `cat message.txt` is shorter, needs nothing taught, and was the answer the
// card handed over in its last hint anyway. So the word now arrives *inside a
// script the student did not write*: a hundred-odd lines of report generator,
// not one character of the word written in it, everything the word is made of falling
// out of the log data as the script does its ordinary work. Finding it is
// level 4's `grep -l`, reading it is level 2's `cat`, and running it is this
// level's `chmod +x` and `./` — the hunt is the only place in the game where
// three earlier levels have to be used at once, and the only one where the
// obvious move (read the file) is a dead end.

// huntScript is the flag's script: a hundred-odd lines of plausible-looking
// report generation in which *no character of the word is written down anywhere*.
//
// One label discipline point, because the level's whole premise is that this
// report looks like something a real nightly job prints: it used to print
// `stages` twice, far enough apart on the page that no reader connects them, with
// different values under each — the header
// spelled the three stage names out and the footer counted them. Two facts, one
// label, and a student who trusts the header concludes the script is lying to
// them. The footer says `by stage` instead.
//
// The mechanism is one helper, `glyph`, which reduces a number modulo 37 and
// indexes a 37-character alphabet. It is called dozens of times, and twenty-odd
// of those calls spell the ordinary words the report prints ("sweep", "rotate",
// "verify", "keep") — so the call shape is the file's background texture. The
// word then arrives as a *by-product of the report's own work*: each log row
// carries a numeric `code` column of the kind any checksum-bearing log has, the
// row's status letter is `glyph "$code"`, and the same value is appended to
// `sig` as the loop goes by. `sig` is printed as the last field of the report
// like any other summary.
//
// Nothing is spelled in octal, nothing is base64'd, and there is no line that
// looks like a hidden string — the numbers are all in one narrow 38–143 band, so
// any single one of them is as meaningless as the next. `cat` on this scrolls a
// hundred-odd lines of `while` loops and counters past with the word nowhere in
// it, and the screen scanner — which wants the whole token — stays quiet until
// the script runs. A student who does read the output carefully can also just
// read the status column top to bottom, which is a reward rather than a leak: it
// still takes a running script to see.
//
// Only `printf`, `sed`, `tr` and `wc` are used — the first is a shell builtin and
// the rest are coreutils — so there is nothing here that a bare workstation could
// be missing. It is also POSIX enough to run under `sh`, which is what the test
// uses to check the output without depending on the execute bit.
const huntScript = `#!/bin/bash
# nightly report for the lab folder

ALPHA='-abcdefghijklmnopqrstuvwxyz0123456789'
COLS=62
SEP='-'

glyph() {
    n="$1"
    while [ "$n" -ge 37 ]; do
        n=$((n - 37))
    done
    printf '%s' "${ALPHA:$n:1}"
}

rule() {
    s=''
    i=0
    while [ "$i" -lt "$COLS" ]; do
        s="$s$SEP"
        i=$((i + 1))
    done
    printf '%s\n' "$s"
}

pad() {
    s="$1"
    n="$2"
    while [ "${#s}" -lt "$n" ]; do
        s="$s "
    done
    printf '%s' "$s"
}

trim() {
    printf '%s' "$1" | tr -d '\r' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//'
}

rows="09:12 sweep 3 49 3 old files removed
09:13 sweep 0 83 nothing to do
09:14 sweep 1 125 1 lock file removed
09:15 sweep 0 58 0 old files removed
09:16 sweep 4 98 4 old files removed
09:17 sweep 2 123 2 old files removed
09:18 rotate 1 38 log rotated
09:19 sweep 9 76 9 old files removed
09:20 verify 1 111 checksum rechecked
09:21 sweep 5 50 5 old files removed
09:22 sweep 3 105 3 empty dirs removed
09:23 sweep 1 122 1 old file removed
09:24 verify 1 67 checksum rechecked
09:25 sweep 7 92 7 old files removed
09:26 rotate 1 143 log rotated"

rule
printf 'nightly report\n'
printf 'folder   : %s\n' "$(trim '.')"
printf 'alphabet : %s symbols\n' "$(printf '%s' "$ALPHA" | wc -c | tr -d ' ')"
printf 'rows     : %s\n' "$(printf '%s\n' "$rows" | wc -l | tr -d ' ')"
printf 'stages   : %s%s%s%s%s %s%s%s%s%s%s %s%s%s%s%s%s\n' \
    "$(glyph 93)" "$(glyph 134)" "$(glyph 42)" "$(glyph 79)" "$(glyph 127)" \
    "$(glyph 129)" "$(glyph 52)" "$(glyph 94)" "$(glyph 112)" "$(glyph 57)" "$(glyph 79)" \
    "$(glyph 59)" "$(glyph 79)" "$(glyph 129)" "$(glyph 46)" "$(glyph 80)" "$(glyph 136)"
rule

total=0
quiet=0
swept=0
turned=0
checked=0
first=''
last=''
sig=''

while IFS= read -r line; do
    time="${line%% *}"
    rest="${line#* }"
    act="${rest%% *}"
    rest="${rest#* }"
    n="${rest%% *}"
    case "$n" in
        '' | *[!0-9]*) n=0 ;;
    esac
    rest="${rest#* }"
    code="${rest%% *}"
    what="${rest#* }"

    total=$((total + n))
    if [ "$n" -eq 0 ]; then
        quiet=$((quiet + 1))
    fi
    case "$act" in
        sweep) swept=$((swept + 1)) ;;
        rotate) turned=$((turned + 1)) ;;
        verify) checked=$((checked + 1)) ;;
    esac
    if [ -z "$first" ]; then
        first="$time"
    fi
    last="$time"
    sig="$sig$(glyph "$code")"

    printf '  %s  %s %s %s %s %s\n' "$time" \
        "$(pad "$act" 7)" "$(pad "$n" 3)" "$(pad "$code" 4)" \
        "$(pad "$what" 26)" "$(glyph "$code")"
done <<EOF
$rows
EOF

rule
printf 'moved    : %s files\n' "$total"
printf 'quiet    : %s rows\n' "$quiet"
printf 'by stage: %s %s %s\n' "$swept" "$turned" "$checked"
printf 'window   : %s to %s\n' "$first" "$last"
printf 'next     : %s\n' "$(glyph 85)$(glyph 116)$(glyph 42)$(glyph 90)"
rule
printf 'signature: %s\n' "$sig"
`

var level5 = &level{
	num: 5, name: "executables", emoji: "🛠️",
	title: "YOUR FIRST SCRIPT", tagline: "your first script",
	lead: "every command is just a file. here is one, already written — all it is missing is permission to run.",
	cmds: []string{"cat", "chmod +x", "./", "PATH"},
	used: []string{"cat", "chmod", "ls -l", "./", "echo", ">", "PATH"},
	seed: []plant{
		{at: "hello", text: "#!/bin/bash\necho \"hello from a file\"\n", mode: 0o644},
		{at: "about.txt", text: `hello is a file. so is ls. so is everything you have typed today.

run one with ./hello once it has the execute bit set.
`},
	},
	sections: []section{
		{
			title: "what is inside it",
			lead:  "a script is a file whose contents are commands. that is the whole trick.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "look inside a file you have never made",
					note: "`cat hello` — it is an ordinary text file with two lines in it. the first starts with " +
						"`#!`, which is how the system knows which interpreter should run it. read it, then press Enter.",
					hints: []string{
						"`cat hello`",
						"there is nothing hidden in here — it is text, exactly like the notes you read in level 2",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "look at its permissions",
					note: "`ls -l hello` — the mode column ends in `-`, not `x`. read it, then press Enter.",
					hints: []string{
						"`ls -l hello`",
						"the last letter of the mode is the one that matters here",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					goal: "give `hello` permission to run",
					note: "`chmod +x` turns on the x (execute) bits",
					hints: []string{
						"`chmod +x hello`",
						"`ls -l hello` again — the mode now ends in x",
					},
					done:  "the x bits are on — it is runnable now.",
					check: isExecutable("hello"),
				},
				{
					kind: kindInfo,
					goal: "see the new x's",
					// `ls -l`, not `ls -l hello`: the note asks the student to
					// compare `hello` against `about.txt`, and a listing of one
					// file cannot show both — the comparison was impossible.
					note: "`ls -l` — the whole folder this time, so you can compare `hello` with `about.txt` next to it. one is runnable, one is not. read it, then press Enter.",
					hints: []string{
						"`ls -l`",
						"one mode ends in x, the other still ends in -",
					},
					done: "seen it — moving on",
				},
			},
		},
		{
			// This used to sit *after* the section that runs the script, which asked a
			// question the student no longer had: the ✅ for `./hello` landed first
			// and the explanation of why the bare name failed came afterwards as
			// trivia. The failure has to come before the win, so the level now runs
			// chmod → it still will not run by name → why → run it.
			title: "why not just `hello`?",
			lead:  "a small experiment that explains a whole system.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "try running it by name",
					note: "`hello` is runnable now and it is *right here* — but type the bare name and see what bash says. read it, then press Enter.",
					hints: []string{
						"just type `hello` and press Enter",
						"`bash: hello: command not found` — then press Enter again to move on",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "see where bash looks for commands",
					note: "`echo $PATH` — those are the folders it searches, in order. read it, then press Enter.",
					hints: []string{
						"`echo $PATH`",
						"this folder is not in that list — which is exactly why the bare name failed",
					},
					done: "seen it — moving on",
				},
			},
		},
		{
			title: "run it",
			lead:  "one command runs one program. `./` is how you say *this one, right here* — which is the missing step from a moment ago.",
			steps: []step{
				{
					kind: kindTask,
					goal: "run your tool and save what it says into `out.txt`",
					note: "`./` means *right here in this folder*",
					hints: []string{
						"`./hello > out.txt`",
						"`cat out.txt` — the tool's own words, in a file",
					},
					done:  "your own command ran and printed what was inside it.",
					check: all(isFile("out.txt"), contentContains("out.txt", "hello")),
				},
			},
		},
		{
			title: "find the flag",
			lead:  "the last beat of the level: a magic word, hidden somewhere here.",
			steps: []step{
				{
					kind: kindFlag,
					goal: "a few new files have appeared in this folder — one of them is a script, and you have not run it",
					note: "I dropped a few things here while you worked. one is a *script* — the same kind of file as the " +
						"`hello` you ran a moment ago — and it will not hand its secret to a reader. run it, and " +
						"the word turns up. whatever puts the word on your screen counts. it looks like " +
						"`linuxlab-…` — I'll spot it the moment it appears. nothing to type back to me.",
					hints: []string{
						"new names in this folder — `ls` first, that is the habit",
						"every script's first line names the interpreter it wants, and `grep -R` looks in here rather than just at one name: `grep -Rl bash .` finds them both",
						"reading this one will not help — the file is written so the word cannot be read off it. running it is the whole point: `chmod +x start`, then `./start`",
					},
					done: "your unread script had the last word.",
					// Nothing on disk holds the word in any form, and there is nothing left to
					// undo: the letters are *derived*, not encoded. One helper reduces a number
					// from the log down to a symbol, and the status letter each row prints is
					// also appended to a signature that is printed last. So there is no escape
					// sequence to reverse and no assignment to read — running the report is the
					// only way to get the word. Hence `assembled`, and hence no test asserts
					// anything about the source: rename the helper and the level is unchanged.
					assembled: true,
					plant: []plant{
						{at: "scratch.txt", text: "reminders for myself:\n  - the printer is on floor 2\n  - friday's lab is cancelled\nnothing urgent.\n"},
						{at: "cleanup.log", text: "Mon 09:12 sweep: 3 old files removed\nMon 09:13 sweep: nothing to do\nMon 09:14 sweep: 1 lock file removed\n"},
						{at: "inventory.csv", text: "item,count\nchairs,12\ntables,4\nprojector,1\n"},
						{at: "start", text: huntScript, mode: 0o644},
					},
					token: "linuxlab-m4k3r5",
				},
			},
		},
	},
}

// ── Level 6 · permissions · chmod u/g/o±rwx then 600/640/700 · id ─────────────
//
// The letters come first. `chmod g+r` says *who* and *what* in the same breath,
// and a student who has that can read `-rw-r--r--` without being told; the
// numbers are then just the letters added up, introduced as the short form
// rather than as the real thing. The vault is the same `chmod 700` either way.
//
// The middle column is only *named* until §5, where `id` supplies the name: the
// fourth column of the `ls -l` the student has been reading all level is the
// same word as the `gid=` on screen, which makes "a group is just a name for a
// set of users" checkable rather than abstract. §1 therefore says "the group
// that owns it" rather than "your group" — `g` is the file's group, which on
// this machine happens to be yours because you made every file in it.

var level6 = &level{
	num: 6, name: "permissions", emoji: "🔐",
	title: "LOCKS & KEYS", tagline: "locks & keys",
	lead: "who may read, who may write, who may run — and who are you?",
	cmds: []string{"chmod u/g/o ± rwx", "chmod 600/640/700", "ls -ld", "id"},
	used: []string{"ls -l", "ls -ld", "cd", "cat", "id", "chmod"},
	seed: []plant{
		{at: "data.txt", text: "nothing to see here\n", mode: 0o644},
		{at: "notes.txt", text: "a shared note\n", mode: 0o666},
		{at: "public.txt", text: "read me\n", mode: 0o600},
		// 0600, not 0755: a folder with no `x` withholds from its *owner*, so
		// the vault is a real lock rather than a gesture. At 0755 the flag was
		// readable before the level's `chmod` and after it, and the climax was
		// built on a state change the player could not observe — or believe.
		//
		// The order of these two lines is load-bearing. `putMode` writes the file
		// through its parent folder, and a 0600 folder has no `x`, so writing into
		// it fails. Writing the file first lets `MkdirAll` make the folder 0755
		// long enough to write into; the `putDir` below then closes it to 0600.
		// `TestAPlantInsideASealedFolderComesFirst` holds that in place.
		{at: "secret/flag.txt", text: "the key was here all along: linuxlab-k3y5\n", mode: 0o644},
		{at: "secret", dir: true, mode: 0o600},
	},
	sections: []section{
		{
			title: "read the mode",
			lead:  "nine boxes in one column: three groups, three letters each. read it before you change it.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "see the permissions of what is here",
					note: "`ls -l` — every line starts with a mode: nine letters, like `-rw-r--r--` on " +
						"`data.txt`. the `secret/` line starts with `d` instead of `-`, because it is a folder. " +
						"read it, then press Enter.",
					hints: []string{
						"`ls -l`",
						"the three groups are you, the group that owns the file, and everyone else — in that order",
					},
					done: "seen it — moving on",
				},
				{
					kind:  kindInfo,
					noCmd: true,
					goal:  "the mode, spelled out",
					note:  "nothing to run this time. nine letters, read left to right: `u` you, `g` the group that owns it, `o` everyone else — and inside each, `r` read, `w` write, `x` execute. so `-rw-r--r--` says: you may read and write it, its group and everyone else may only read it. a `-` where a letter would be is the absence of that permission. (what a group *is* is two sections away — `g` is just a slot here.) press Enter when you've read it.",
					hints: []string{"nothing to run — press Enter"},
					done:  "seen it — moving on",
				},
			},
		},
		{
			title: "move one letter at a time",
			lead:  "`chmod` also speaks in those letters — handier when you only want to move one box.",
			steps: []step{
				{
					kind: kindTask,
					goal: "take your own write permission away from `data.txt`",
					note: "a letter says *who*, a `+` or `-` says which way, and the letter after that says *what*",
					hints: []string{
						"`chmod u-w data.txt`",
						"`ls -l data.txt` — only the first group changed: `r--r--r--`",
					},
					done:  "`data.txt` is read-only for you now — `-r--r--r--`.",
					check: modeIs("data.txt", 0o444),
				},
				{
					kind: kindTask,
					goal: "let everyone read `public.txt`",
					note: "you can say \"everyone\" as one letter: `a` is `u`, `g` and `o` together",
					hints: []string{
						"`a` covers all three groups at once",
						"`chmod a+r public.txt`",
						"`ls -l public.txt` — the last two groups both gained an r",
					},
					done:  "`public.txt` is public — `-rw-r--r--`.",
					check: modeIs("public.txt", 0o644),
				},
				{
					kind: kindTask,
					goal: "take the group *and* everyone else's write away from `notes.txt`, leaving your own alone",
					note: "several letters can go in one go: `go` is *g* and *o*, so it is one edit that " +
						"covers two columns",
					hints: []string{
						"`chmod go-w notes.txt`",
						"`ls -l notes.txt` — `rw-r--r--`: the last two columns lost their w and yours did not",
					},
					done:  "`notes.txt` is yours to write and its group's to read.",
					check: modeIs("notes.txt", 0o644),
				},
				{
					kind:  kindInfo,
					noCmd: true,
					goal:  "the same three letters, in numbers",
					note:  "nothing to run this time: each letter is worth a number — `r` is 4, `w` is 2, `x` is 1 — and each group is one digit, so `-rw-r--r--` is just 644. that is where `chmod 644` comes from, and it is the same edit either way. press Enter when you've read it.",
					hints: []string{"nothing to run — press Enter"},
					done:  "seen it — moving on",
				},
			},
		},
		{
			title: "and when you know the letters",
			lead:  "the numbers are quicker once you can read the letters. two files that are already set up for it.",
			steps: []step{
				{
					kind: kindTask,
					goal: "make `data.txt` private again, in digits this time",
					note: "600 means `rw-------`: read and write for you, nothing for anyone else",
					hints: []string{
						"`chmod 600 data.txt`",
						"`ls -l data.txt` — the same as `chmod u+rw,go-rwx` would have given you",
					},
					done:  "`data.txt` is private — `-rw-------`.",
					check: modeIs("data.txt", 0o600),
				},
				{
					kind: kindTask,
					goal: "make `notes.txt` group-readable and nothing more, in digits",
					note: "640 means `rw-r-----`: your group gets read, and nobody else gets anything",
					hints: []string{
						"`chmod 640 notes.txt`",
						"`ls -l notes.txt` — one digit is all it took",
					},
					done:  "`notes.txt` is `rw-r-----`.",
					check: modeIs("notes.txt", 0o640),
				},
				{
					kind: kindInfo,
					goal: "compare the two modes you have made",
					note: "`ls -l` — `data.txt` next to `notes.txt`. read it, then press Enter.",
					hints: []string{
						"`ls -l`",
						"one digit apart in the group column is the whole difference between them",
					},
					done: "seen it — moving on",
				},
			},
		},
		{
			title: "the vault",
			lead:  "a folder's `x` is not the same thing as a file's. it is the difference between looking in and going in.",
			steps: []step{
				{
					// The surprise comes first, and it is the whole lesson: you
					// can read the *names* in a folder you have no right to enter.
					// `r` on a folder is exactly that and no more.
					kind: kindInfo,
					goal: "see what is inside `secret/`",
					note: "`ls secret` — a folder you have not been let into yet, and `ls` " +
						"still lists what is in it. that is `r` on a folder: you may read the *names*. " +
						"read it, then press Enter.",
					hints: []string{
						"`ls secret`",
						"the note is called `flag.txt`, and you can see its name from out here",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "try to read the note inside it",
					note: "`cat secret/flag.txt` — bash says no. you can see the name and you cannot " +
						"have the file, and those are two different permissions on the same folder. read what " +
						"bash says, then press Enter.",
					hints: []string{
						"`cat secret/flag.txt`",
						"permission denied — read it, then press Enter to move on",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "read the folder's own mode",
					note: "`ls -ld secret` — a folder needs `-d`, or `ls` lists what is inside it instead. " +
						"and the mode says why: `drw-------`. your three letters are `rw-`, and the missing " +
						"one is `x`. on a folder `r` lets you read the names inside, and `x` is what lets you " +
						"*go in* and reach the files themselves. read it, then press Enter.",
					hints: []string{
						"`ls -ld secret`",
						"compare it with the files: `d` instead of `-` at the front, and no `x` in your three letters",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindTask,
					// `chmod +x` is *not* the answer and the hint says so: on a 0600
					// folder it gives 0711, handing the right to go in to group and
					// other as well. Verified, not assumed — the climax is no place
					// to discover that the obvious command misses the check.
					goal: "give `secret/` back the `x` it is missing",
					note: "`x` on a folder is the right to *traverse* it: to go in, and to reach the files " +
						"inside. take the whole letter for yourself while you are there",
					hints: []string{
						"`chmod 700 secret` — `rwx` for you, nothing for anyone else",
						"`chmod u+x secret` also works; plain `chmod +x` would hand the key to everyone",
					},
					done:  "there is an `x` in your three letters now — you can go in.",
					check: modeIs("secret", 0o700),
				},
			},
		},
		{
			title: "you, and your groups",
			lead:  "every lock you have set so far comes down to two questions: who you are, and which groups you are in.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "ask the machine who you are",
					note: "`id` — three fields to read: `uid=` is your user number, `gid=` is one group you belong " +
						"to, and `groups=` is *every* group you are in, which is usually a longer list. read it, " +
						"then press Enter.",
					hints: []string{
						"`id`",
						"three fields: `uid`, `gid`, and the list after `groups=` — the list runs on for a while",
					},
					done: "seen it — moving on",
				},
				{
					kind:  kindInfo,
					noCmd: true,
					goal:  "so what is a group, then?",
					note: "nothing to run this time, just read. a *group* is only a name for a set of users, and every file " +
						"has one: the fourth column of your `ls -l` is that name — the same one as the `gid=` above. " +
						"that is why `-rw-r--r--` has a middle column at all. `r--` there says *anyone in that " +
						"group may read this*, and the machine never needs your name. press Enter when you've " +
						"read it.",
					hints: []string{"nothing to run — press Enter"},
					done:  "seen it — moving on",
				},
				{
					kind:  kindInfo,
					noCmd: true,
					// This used to ask "why is there no `sudo` on this machine?",
					// which is a claim about the *host*: true on the Arch
					// workstations, false on the VM's Ubuntu, wrong again on
					// anyone's laptop. The game is not tied to the WS, so the card
					// states the part that is true everywhere — root is not bound by
					// the permission bits — and drops the claim about the machine.
					// The uid anchor stays: it is what makes the level's own
					// `chmod 700 secret` mean something.
					goal: "so what does `root` get to do?",
					note: "nothing to run this time, just read: `root` is user number 0, and it is not bound by " +
						"the permission bits at all — it can read, change or delete any file whatever the mode says. " +
						"your `uid` is an ordinary number, so the bits on `secret/` applied to you as well: that " +
						"folder turned *you* away until you gave it an `x`. press Enter when you've read it.",
					hints: []string{"nothing to run — press Enter"},
					done:  "seen it — moving on",
				},
			},
		},
		{
			title: "the vault opens",
			lead:  "you made the key. now use it.",
			steps: []step{
				{
					kind: kindFlag,
					goal: "you gave `secret/` the `x` it was missing — go in and take what is inside",
					note: "that missing letter was the whole lock: until you added it, the folder turned " +
						"you away too, and not even `root` would have needed permission. it looks like " +
						"`linuxlab-…` — I'll spot it the moment it appears on your screen. nothing to type " +
						"back to me.",
					hints: []string{
						"the folder is right here, and the letter you added is the key",
						"step inside and read the note that was waiting for you",
						"`cd secret`, then `cat flag.txt`",
					},
					done:  "opened, and yours.",
					token: "linuxlab-k3y5",
				},
			},
		},
	},
}

// ── Extra · habits · man --help Tab ↑ Ctrl-C Ctrl-D ──────────────────────────
//
// Not a level: the habits toolbox. It has no number and no flag, and it never
// runs as part of the sequence — the menu is the only way in, and finishing it
// returns to the menu. It is the home of the small shell habits the tour leans
// on, given a place to practise them without pretending to be a sixth family.

var habitsLevel = &level{
	num: 0, extra: true, name: "habits", emoji: "🧰",
	title: "HABITS & THE MANUAL", tagline: "habits & the manual",
	lead: "the small moves that make a shell feel like home.",
	cmds: []string{"man", "--help", "Tab", "↑", "clear", "Ctrl-C", "Ctrl-D"},
	used: []string{"man", "--help", "Tab", "↑", "clear", "Ctrl-C", "Ctrl-D"},
	seed: []plant{
		{at: "secret/note.txt", text: "no flag lives here.\njust habits — make them yours.\n"},
	},
	sections: []section{
		{
			title: "the manual",
			lead:  "every command carries its own instructions.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "read the manual for `ls`",
					note: "`man ls` opens the manual inside the terminal: read a little, then press `q` to quit.\n" +
						"press Enter when you have seen it.",
					hints: []string{
						"`man ls`, read a bit, then press `q`",
						"`q` leaves the manual — that is the trick to remember",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "ask `ls` for its own help",
					note: "`ls --help` — the short version, straight on screen. read it, then press Enter.",
					hints: []string{
						"`ls --help`",
						"man pages are long; `--help` is the quick glance",
					},
					done: "seen it — moving on",
				},
			},
		},
		{
			title: "the shell's memory",
			lead:  "the keyboard does half the typing for you, if you let it.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "let Tab finish a name for you",
					note: "type `cd sec` and press Tab — the shell completes the folder name. read it, then\n" +
						"press Enter.",
					hints: []string{
						"`cd sec` then Tab",
						"Tab completes `sec` to `secret/` — then Enter walks in",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "call back the last command",
					note: "run `ls`, then press ↑ — the shell remembers. read it, then press Enter.",
					hints: []string{
						"type `ls`, press Enter, then press ↑",
						"↑ walks back through what you have typed; Enter runs it again",
					},
					done: "seen it — moving on",
				},
				{
					kind: kindInfo,
					goal: "wipe the screen when it gets noisy",
					note: "`clear` empties it — and ↑ still calls your old commands back, so nothing is lost.\n" +
						"read it, then press Enter.",
					hints: []string{
						"`clear`",
						"the rows go, the history stays where ↑ finds it",
					},
					done: "seen it — moving on",
				},
			},
		},
		{
			title: "the keys",
			lead:  "two keys that get you out of anything.",
			steps: []step{
				{
					kind: kindInfo,
					goal: "stop a running command",
					note: "run `sleep 60`, then press Ctrl-C — the command dies on the spot.",
					hints: []string{
						"`sleep 60`, then Ctrl-C",
						"any stuck command stops with Ctrl-C — nothing else needed",
					},
					done: "seen it — moving on",
				},
				{
					kind:    kindInfo,
					exitKey: true,
					goal:    "know the way out",
					note: "Ctrl-D is *end of input* — bash takes it as *leave, please*.\n" +
						"this time the game asks before letting you go. try it.",
					hints: []string{
						"press Ctrl-D, then read what the game asks",
						"`y` leaves · anything else stays — both are safe to try",
					},
					done: "the exit key is yours now.",
				},
			},
		},
	},
}

// ── Scaffolds ────────────────────────────────────────────────────────────────
// Each level's playground is wiped and rebuilt from its seed, so a run (or a
// -level N rehearsal) always starts from the same clean state. What a step
// plants later — the flag hunts — lives on the step, not here.

// scaffold builds the level's playground from scratch: the level's seed, and
// nothing else. Folders get an explicit mode: the umask must not quietly
// satisfy a step (e.g. `chmod 700 secret`).
func (lv *level) scaffold(dir string) error {
	if err := plantAll(dir, lv.seed); err != nil {
		return err
	}
	return os.Chmod(dir, 0o755)
}
