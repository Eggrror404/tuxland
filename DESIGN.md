# tuxland — design (locked)

The guided command-line game. Players type **real bash** on the **real filesystem**;
the game supervises (baby-sit style) and never parses what they type. This document
is the single source of truth for the build — philosophy in §1, mechanics in §2–§4, the
screen in §5, the level content and the reasoning behind it in §7, and how it is
verified in §8. Expected reader: a fresh agent session that will write the code.

Related doc: `README.md` (player-facing: install, build, dependencies, the `DUMP` /
`TUXLAND_REPLAY` dev tools).

**On "the deck".** tuxland was written for a Linux-introduction workshop whose slide
deck lives in a separate repository that is not shipped here. Wherever this document
says *the deck*, it means that deck: its tour families set the level order and the
numbering, and the contract in §1.1 is what keeps the two in step. The reasoning that
cites it is kept, because it is the reason the content has the shape it does.

---

## 1. What it is (decided)

- **Guided on-ramp** for people who have never used a shell — hand-held levels, hints
  on mistakes, ease of follow-along. **NOT a hard CTF.**
- **Real bash + real coreutils + real filesystem** on the player's own box. No
  simulated shell or interpreter anywhere, and none is planned (§9).
- **Name:** tuxland — Tux, the Linux penguin, is the mascot and the brand. The repo and
  the binary follow the name; 🐧 is the game's face.
- **Deploy target:** anything with a `bash` on the `PATH` — the workshop's Linux
  workstations (Arch, ssh session, no sudo), a laptop, WSL, macOS. **No cross-platform
  packaging or testing work** (§9); what that costs is §1.2's rule, that nothing may
  assume a particular host, and that rule is also why one build runs everywhere.
  Offline play is not required.
- **Exit ramp:** after the last level, point finishers at **OverTheWire Bandit** and
  **pwn.college**.

### 1.1 The deck ↔ game contract

**Slides carry the concepts, the game carries the commands.** The deck (external — see
the note at the top) is concept-oriented — what a command *is*, why the system works
that way, when you would reach for it — and deliberately holds less command/flag/example
detail than it once did. The game is where attendees **use** those commands, so it owns
the concrete half outright: a task card names the file or folder its check grades, a flag
card names the place and the challenge without giving the command away, and the hint
ladder escalates until the last hint hands over the exact command to type.

Three rules follow, and they are rules for the *writing* rather than for the code:

- **The game may never assume the slides already listed a command.** A student who
  missed the session, or who is playing from home weeks later, gets everything from the
  cards and the hints — that is the only complete reference they have.
- **The hint ladder is the only place a command is spelled out** for a flag hunt, so
  the flag card itself has to stay above it (§4, §7).
- **The deck's tour slides and the game's levels stay in the same order under the same
  family names**, so the live demo narrates the slides instead of repeating them. The
  game *is* the showcase of the deck's CLI tour.

**The game's command coverage is a superset of the deck's.** Every command a tour slide
names must be something a student *uses* here — the same command, with a beat that
grades or teaches it. Flags and forms count as usage, because they are what people get
wrong (`ls -la`, `rm -r`, symbolic `chmod`, `tee` all appear in slides, so all four have
a beat here). A slide may also name a command only as a way to make a point — the
deck's `which cat` shows that `cat` is a file under `PATH`, and is not a sixth thing to
learn — and where a deck command is deliberately *not* used, say why: `less`/`vim`
(reading is the IDE workshop's job) and `scp` (there is no ssh inside the playground).

### 1.2 Rules that cut across levels

- **One command per beat.** Going past the deck is allowed when a command needs a
  playground to be worth teaching — L4 adds `find` and `grep -R`, which the tour never
  names, because both need a *tree* and the tour only ever has one log in frame. The
  corollary is that a step never chains two commands the student has not met.
- **A hunt should land the student somewhere they have never been.** L3 plants an
  `inbox/` that did not exist a minute ago; L4 plants a year that was not in the tree
  when the student listed it. Two reasons, the second being the real one: the hunt stops
  being a re-read of what the level just did, and it cannot be spoiled — nothing already
  on the disk holds the word, so no earlier beat, hint or idle `cat` can give it away.
  The levels whose flag is *meant* to be sitting in a file they must go and find — 2, 5,
  6 — deliberately do the opposite; L5 is the exception inside that exception, because
  its word is in no file at all (§7).
- **No card may assert something about the host it runs on.** The game is not tied to
  the workstations: L6 §5 used to ask "why is there no `sudo` on this machine?", true
  on the Arch workstations, false on the VM's Ubuntu and wrong on a laptop. The deck
  *may* say "no sudo on the WS", because that is a true fact about the session; a game
  card may not. (The same rule is why the toolbox's `man` card says nothing about what
  `which` prints for a name that is not a command — implementations differ.)

Per-level decisions live in §7, with §8 for how all of this is verified.

## 2. The game mechanic (decided: hybrid)

Each level = **a few guided sections, ending in a flag hunt** (the multi-section layout
so a level never feels crowded). Two step kinds plus the finale:

| Step kind | What happens | How the game checks it |
|---|---|---|
| `info` | "Run this, look at the output" (`pwd`, `ls -l`, `man`, `echo $PATH`, …). The game prints a command box; the student runs it in bash and presses Enter to continue. | no check — waits for Enter |
| `task` | "Make this state happen" (`touch`, `mkdir`, `mv`, `cp`, `echo >`, `chmod`, …). | **artifact check** against the real FS (exists / isDir / contents / mode), after each submitted line and on a slow idle poll. Pass → auto-advance "✅". Fail → keep waiting, auto-hint after a couple of misses, gentle time-based nudges if the student freezes. |
| `flag` (finale) | Riddle: find the magic token (`linuxlab-…`) hidden in the level, so that reaching it *requires* the level's commands. | see §4. |

**Why not parse commands:** to keep tab-completion, arrows, history and colours
authentic, the game relays keystrokes verbatim to a pty bash — it cannot see *what* was
typed and deliberately does not try. All supervision is state-based (what is on disk,
what token appeared on screen), never content-based.

**No quizzes of any kind** — every successful check is a real FS state or a token
rendered by a real command.

## 3. Architecture (decided)

- Go module under a thin CLI (§6). Deps: `github.com/creack/pty`, `golang.org/x/term`;
  everything else stdlib. `go.mod` asks for the Go version `x/term` needs as its floor,
  not whatever toolchain happens to be installed.
- A real bash is spawned **inside a pseudo-terminal** (`--norc --noprofile -i`), with the
  level folder as its working directory and a plain custom `PS1`. Our stdin goes raw and
  every keystroke is forwarded to the pty verbatim, so Ctrl-C, arrows and tab reach bash
  untouched; pty output is relayed to the screen.
- **The Enter boundary is bash's own prompt.** The game runs the current step's
  verification when the relay sees the `PS1` marker — the shell saying it has nothing
  left to say — with a slow idle ticker only as a backstop for a bash that will never
  prompt again. The first build guessed with a settle delay plus a quiet gap, which put a
  visible pause on every command and drew the game's line *above* output that had not
  arrived yet; the signal was already being read and thrown away. Answering on the marker
  costs nothing the student can see, for a silent command and a chatty one alike, and the
  fallback is set far longer than any command the game asks a player to run so it can
  only cost a late prompt, never a wrong screen.
- **`PS1` is a single invisible marker and the game draws `tuxland$ ` itself**, at the end
  of every block that precedes typing — including after a command the game has nothing to
  say about. A real `PS1` collides with the ✅ line and risks injecting a newline into a
  half-typed command, and the relay strips the marker so there is always exactly one prompt
  on screen. Readline silently swallows `PS1=U+0001`, so the marker is U+2060 (a word
  joiner: invisible, and stripped anyway). The marker doubles as the "bash is up" signal —
  keystrokes that reach a pty before bash has set its own termios get flushed by it — so the
  game waits for the shell's first prompt before printing a level's first card. That first
  marker raises no event, because the card answers for it by drawing its own line
  underneath; **every prompt after it owes the student a line**, including one the student
  made alone (Ctrl-C, Ctrl-L) — the answer is "reported as `^C` and then nothing, but typing
  still works".
- **One keyboard for the whole run** (`keyboard.go`), not one stdin reader per level. A
  reader per level is a race: the reader left over from the previous level is still
  parked on stdin, so the first line of a new level could be written to a shell that had
  just died and vanish — which happened once per level. Keystrokes that arrive between
  two shells are held and dropped when the next level opens, so a stray Enter cannot skip
  step one. Raw mode is therefore also switched once per run, and no stale reader can hand
  back a terminal it no longer owns.
- **Hints:** automatic — no `next`/`hint` keywords. The first failed check prints
  nothing (bash's own error already told them); hints start on the second or third. A
  time-based nudge rescues a student who has frozen without typing at all.
- **Escape line: after 6 submitted lines on one card, once**, the game names the only
  universal exit — `exit` and start the level again, because the playground is wiped and
  rebuilt. It exists because watching only the disk makes the game trustworthy *and*
  blind: if the student has already moved, renamed or deleted the thing a card looks for
  (`mv first.txt dnoe/` on a card that wants it at the root, `rm notes/ideas.txt` before
  the hunt), then no command they can type will ever pass, the hint ladder cycles
  forever, and the screen repeats itself. A **flag hunt is the worst case** — it says
  nothing on any attempt at all, so a deleted flag was silence until the nudge — so the
  escape line is wired into that path too. The game cannot tell "broken" from "stuck",
  and diagnosing would mean parsing their input, which it will not do; so it states the
  one thing true either way and needs no diagnosis. Counted in **submitted lines, not
  seconds**, deliberately: a student who thinks for ten minutes and then types once is
  not stuck and must not be told to give up. Six submissions of something that did not
  work is. Once per card (`st.rescued`), and an info card — at most two presses — can
  never reach it.
- **Flags:** the game scans the pty→screen output stream for the level's token while the
  flag step is active; when the token appears (the student ran the right command and it
  printed) the game celebrates and advances. No typing-it-back ceremony, and the token
  must **never** be printed by the game itself — not in text, not in a hint.
- **Scratch dir:** `<lab root>/level-NN-name/` per level, **wiped and rebuilt** when a
  level starts, so re-running with `-level N` is a reset. Nothing outside it is touched.
  No sudo. `LINUXLAB_ROOT` makes the root relocatable for tests.
- **Progress:** finished levels are remembered in `<lab root>/progress.json`, written the
  moment a level's flag is found (atomically: a temp name, then a rename), so a run cut
  short still counts. The opening menu shows a ✅ per finished level, a one-line count,
  and puts its cursor on the first unfinished one. The file is a convenience and nothing
  more — an unreadable or hand-mangled one means "start anywhere", a failed write is
  shrugged off, and `rm -rf <lab root>` still resets everything the game knows.
- **Controls** (flags only, no in-game keywords): `-level N` skips the menu and is also
  the rehearsal path; `--list` prints the level table. A default run opens the **level
  menu** — pick with ↑/↓ or a digit and Enter — and then plays level N→the last in
  sequence with a "press Enter for the next level" break (auto-continue when stdin is not
  a TTY). The menu also carries the unnumbered **✦ habits** toolbox (§7), which is never
  part of the sequence, which `-level` cannot reach, and which returns to the menu when
  finished. After the last numbered level: the exit screen with Bandit / pwn.college.
- **Exits:** terminal raw mode is always restored (defer), winsize is propagated on
  SIGWINCH; the student leaves with `Ctrl-D`/`exit` and the game ends gracefully; stdin
  EOF (piped or scripted) ends cleanly too.
- **Closing the pty master does not hang bash up**, so `session.close` sends `SIGHUP`
  itself, falling back to `SIGKILL`. Without it a level transition stalled on a wait
  nobody could see why, and left a stray shell holding the level's folder.

## 4. Flag mechanic detail

- Token format: `linuxlab-` + lowercase letters/digits (the leetspeak suffix is
  on-brand, e.g. `linuxlab-h1dd3n`). Six tokens, one per numbered level; the habits
  toolbox has none — there is nothing to find there, only habits to build.
- Trigger: while a flag step is the active step, the relay scans incoming chunks for the
  token and fires once. The practical consequence is that the student must *run a command
  that prints the flag*; seeing the file name is not enough, it must be rendered.
- **Hints escalate, they never answer.** The card asks for the thing the level just
  taught and says *what* the word is and where it hides, never which command gets it
  out; the first hint narrows it ("only one of those lines is longer than the others"),
  the second points at the place, and only the last hands over the command
  (`grep ERROR access.log`). Spelling the command out on the card taught nothing — the
  student had just learned the level's commands and was being told which one to pick.
  This is a line the *writing* has to hold rather than a test (§8).
- A flag step has no `check` (nothing to grade — the token is caught on screen), so its
  hints arrive on the **time-based nudge** instead of a miss, every 50 s. That is what
  makes the ladder worth keeping: the answer is two minutes away, and a student who was
  going to find it never waits for it.
- No hint and no note ever contains the token value.

## 5. Interface (locked)

**One terminal, one stream.** Real bash output and game text share the screen — no split
panes, no TUI library, ANSI only. The active "card" always sits **directly above the
bash prompt**, so students never hunt for the current task. Bash lines stay plain; game
lines get a distinct voice (bold/colour + symbols) so the two never blur.

**Rules of thumb**

- First failed try = silence (bash's own error already told them); hints appear after a
  couple of tries; a time-based nudge rescues frozen students. Hints always re-echo the
  goal compactly, so nobody has to scroll back to find the task.
- "✅ / ⚠" not "correct/wrong" — nothing here should feel like failing a test.
- No full-screen clears mid-session (jarring); every card stays ≤ ~8 lines.
- Symbols: `▶` task · `⚠` not-yet + hint · `✅` done · `🎉` flag/level win · `🏁` flag
  section · `── §` section header · `═══` level banner.
- Colours only when stdout is a TTY — piped and scripted runs stay clean.
- Prompt = coloured `tuxland$ `, so "the shell talking" is visually separate from "the
  game talking".
- End of level the game prints *what the student can now do* (the used commands) — a
  small win-summary, not a grade.
- **A star inside backticks is a literal star.** Backticks are the "this is a real
  command" voice and they win, so `-name "*.log"` prints as written; `*emphasis*` is a
  voice of the prose *around* the commands. One pass used to handle both, which ate the
  star and had a card — and a hint — tell the student to run `-name ".log"`.

**Everything the game draws is laid out against the terminal's real width**, re-read on
each use (`term.GetSize`, falling back to 80): the student splits, resizes and maximises
the window mid-level, and a fixed-width card would fold into the next line and break every
indent under it. The rules that came out of it, all in `ui.go`:

- Columns are measured honestly: an emoji is two, an escape sequence is zero. A hanging
  indent is the *visible* width of the prefix it hangs from, so an emoji-led heading
  still lines up. Widths below 20 are floored — past that the game would be drawing for
  a phone in portrait.
- A wrap hands back verbatim slices with their offsets, so a continuation line is
  re-voiced in the opening line's colour. It is greedy and breaks only on spaces.
- A `` `command` `` is never split by a break — it moves down whole — and a `-flag` is
  never separated from the command in front of it. The one exception is a command wider
  than the line, which breaks like any other over-long word: the terminal would cut it
  there anyway, and refusing would push the overflow into the layout instead of hiding it.
- A label whose value cannot share its line (`your playground: …`, `you just used: …`)
  takes a line of its own, with the value re-laid under it.
- The `-list` table is three columns only when they all fit; below that it is one line
  per level with a dim `tagline · cmds` line under it.
- The only lines allowed to run over are single unbreakable tokens — a URL, an absolute
  path. No layout helps those.
- **The shell's own output is relayed verbatim.** A command whose output is longer than
  the window still runs off the edge and the terminal wraps it; only the game's own lines
  are laid out. Rewriting what bash said would be a lie about what the shell does.
- **The game writes its own carriage returns**, because the keyboard owns the terminal:
  raw mode on stdin is the same device as stdout on a terminal, so the kernel stops
  translating the game's `\n` into CRLF. A bare LF only indexes down a row and keeps the
  column, so every card slides right by the length of the line above it. The bytes stay
  correct, only the screen is ruined — which is why the lock on it is a model of the
  screen and not a check on the stream (§8). Into a pipe there is no cursor and the output
  stays plain text.

**Kickoff** (default run — the level menu; `-level N` skips straight into a level):

```
  🐧 TUXLAND
  the guided command-line game

  How it works
  • you type REAL bash — this is a real shell on real files
  • I watch your workspace; when a step is done I say so
  • stuck? hints appear after a couple of tries
  • Ctrl-D or exit leaves · your finished levels are remembered

  Levels
    ✅ 1  navigation
        where you stand · pwd cd ls ls -a ls -l ls -la
    ✅ 2  files
        make, keep, throw away · cat mkdir touch mv cp rm
      3  text-io
        where the output goes · echo > >> < | tee bc
      4  read-it
        the tools at the end of a pipe · cat wc -l wc -w grep grep -c grep -l
        grep -R find sort uniq
      5  executables
        your first script · cat chmod +x ./ PATH
      6  permissions
        locks & keys · chmod u/g/o ± rwx chmod 600/640/700 ls -ld id
      ✦  habits
        habits & the manual · man --help Tab ↑ clear Ctrl-C Ctrl-D

  2 of 6 levels done · next up: level 3

  ✦ the habits toolbox — no number, no flag: man, Tab, ↑ and the keys that get
  you out.

  ↑/↓ to move · 1-6 to jump · Enter to start — and from there on it is you and
  bash.
tuxland ▶ 3 text-io
```

The `✦` row is the unnumbered habits toolbox (§7). The last line is the choice, and it
is live: ↑/↓ or a digit repaints it in place — a carriage return and padding over the old
line, never an erase sequence, which would need a cursor probe or a hard clear and this
design has neither. The padding tracks the widest line seen, so a shorter one covers the
longer one it replaces, and the width is measured with the colour stripped, because a
measurement that counts escape bytes silently dropped the level name on a window that
fits it. A pipe gets one plain-text line. Enter commits and the level opens beneath it;
Ctrl-D leaves with the progress saved. A burst of keys landing in a single read still
repaints the committed level, so the last thing on the menu is the level being entered.

**Level start + a task with two tries** (typo → gentle hint, not punishment):

```
  ════════════════════════════════════════════
  📁 LEVEL 2 · YOUR FIRST FILES
  files are the whole point of a computer.
  Your playground: ~/linux-lab/level-02-files
  Nothing outside it can break.
  ════════════════════════════════════════════

  ── §1 · read first ─────────────
  before you make anything, read what is already here.

  ▶ read the note
      cat notes/todo.txt prints a file's contents. read it, then press Enter.

tuxland$ cat notes/todo.txt
learn the moves:
  cat  mkdir  touch  mv  cp  rm
tuxland$
  ✅ seen it — moving on

  ▶ make a folder for the things you create
      mkdir = make directory
tuxland$ mkdir don                  ← student typo
tuxland$ mkdir don                  ← again
  ⚠ not there yet — mkdir done
tuxland$ mkdir done
  ✅ done/ is there.
```

**Info beat** (observe → Enter; level 1 is all info — it has nothing to grade):

```
  ▶ see the hidden names too
      ls -a — the -a shows everything, dot-files included. read it, then press Enter.
tuxland$ ls -a
.  ..  .hidden.txt  notes  photos  readme.txt
tuxland$                       ← just Enter
  ✅ seen it — moving on
```

**Flag finale** (key line: *nothing to type back to me*, and the card poses the challenge
without naming the command — level 1's flag **is a name**, so a hidden folder carries it):

```
  ── §3 · find the flag ──────────────
  🏁 something new just appeared here — and its name is the magic word
      the listing you took a moment ago is already out of date: one of the names
      here starts with a dot, and that one is new. the word looks like
      linuxlab-… — I'll spot it the moment it appears on your screen. nothing to
      type back to me.
tuxland$ ls
.  ..  hidden-names-you-cannot-see   ← the plain listing misses the new folder
  … and a minute later, once:
  ⚠ not there yet — the short listing never shows a name that starts with a dot
tuxland$ ls -a
.  ..  .hidden.txt  .linuxlab-h1dd3n  notes  photos  readme.txt
  🎉 that's it — you found the flag! that folder's name was the word.
```

**Level complete / exit ramp:**

```
  🎉 LEVEL 1 COMPLETE
  you just used: pwd · cd · ls · ls -a · ls -l · ls -la · ls --help
  Press Enter for Level 2 (or Ctrl-D to stop here)
…
  🎓 ALL SIX LEVELS DONE
  real commands, real files. now the real thing — same idea, harder:
   • OverTheWire Bandit  https://overthewire.org/wargames/bandit/
   • pwn.college         https://pwn.college/
  start at Bandit Level 0 — you're already ahead of it. 🐧
```

## 6. Why the code is shaped this way

There is no file listing here on purpose — read the directory. These are the four
shaping decisions that the code does not explain on its own.

- **A library under a thin CLI.** The game is `internal/tuxland`, with `main.go` holding
  only the command line: flags, the signal → terminal-restore handler, and calls into
  `List` / `Play` / `Usage`. The tests have to live in the same package as the code they
  reach into — `game`, `askLevel`, `menuKey` and the rest are unexported — and that
  package is the one that holds the game.
- **One writer for the whole screen** (`ui.go`). The pty relay and every card go through
  the same serialized writer, or a ✅ can land in the middle of a half-typed line. Its
  `write`/`withCR` comments carry only the local reason; the carriage-return story and
  the staircase it prevents are §5's, and they point back rather than repeat it.
- **One stdin owner** (`keyboard.go`, §3). Its type comment keeps the local consequence —
  a line written to a shell that had just died, and vanishing — and defers the rest.
- **The tests sit next to the code they check**, split by what they can reach: `ui_test`
  on the buffer, `pty_test` on the layer nothing else can — the real terminal — and
  `replay_test` on the whole game, typed. §8 has the invariants each one locks.

## 7. The six levels + the toolbox (built content)

Levels follow the tour families in order — navigation, files, **text-io**,
**read-it**, executables, permissions — plus the unnumbered **✦ habits** toolbox. The
tour's single *Input & output* family is levels 3 and 4: 3 is the plumbing, 4 is the tools
that run on it, because searching needs the plumbing and the plumbing earns its own level.
So the deck's per-family `#gamelevel` labels carry that numbering, and the wiring slide
says level 3 while the pipes slide says levels 3 & 4 (its fence is a `grep`/`tee`/`wc`
pipeline — level 4's own tools). Level folders are `level-01-navigation` …
`level-06-permissions`, and `level-extra-habits`.

Below, each level gets its shape and then the decisions behind it — the ones that look
like mistakes and are not. The cards themselves are `levels.go`; this is the map, not a
transcript.

### The level API — writing or editing a level

A level is one struct literal: identity (`num`, `extra`, `name`, `emoji`, `title`,
`tagline`, `lead`, `cmds`, `used`), the `seed` (what the playground holds when the level
starts, replanted fresh every run), and `sections` → `steps`. Everything the game puts on
disk, at scaffold time or later, is a `plant`: `at` (a playground-relative path), `dir`
for a folder, `text` for a file, and `mode` when something grades permissions. A level
with `extra: true` has `num: 0`, so it is not in the 1→6 sequence, a digit cannot reach
it, and `next()` never offers it.

Each step is one beat, of three kinds:

- `kindInfo` — look. One Enter runs the command, a second (or a lone one, marked
  `noCmd`) is the "I've read it" tap. Nothing is graded on disk. A step may set
  `exitKey`, which lends the keyboard to the game for that one beat — the toolbox's
  Ctrl-D lesson.
- `kindTask` — do. Graded by a `check` composed from the helpers in `verify.go`. Checks
  read the real filesystem only, never what was typed.
- `kindFlag` — find. Carries the `token` watched for on screen and its own `plant`, which
  goes in the moment the hunt starts, so the flag is findable when — and only when — the
  hunt does.

Prose (`goal`, `note`, `hints`, `done`) is plain text, with backticks marking code spans;
the layout wraps it itself.

**The rules a card has to hold**, which the writing cannot enforce and §8 partly tests:

- **The card names the target, the hints hand over the command.** A task names the file
  or folder it grades, in a code span; "make a folder for the things you create" grades
  `isDir("done")` and says nothing about `done`, which leaves the name only in the hint
  — and the hint is the answer, not the task. The one legal exception is a step acting
  on what the step above it made: "delete it — for good" is unambiguous because the goal
  above named the file.
- **A task must not already be satisfied by the seed**, or the game congratulates a
  student who did nothing.
- **A flag card never names the command** (§4).
- **A task that *modifies* something must check the part that could be lost.** L3's `>>`
  card grades the new line *and* the survival of the old one, so a student who typed a
  single `>` gets stuck on their own mistake instead of being congratulated for it.

### L1 — navigation · `pwd cd ls ls -a ls -l ls -la` (flag: a folder's name)

Pure navigation — nothing is created or read by name, and `ls` is introduced as the way
to look. Scaffold: `readme.txt`, `notes/readme.txt`, an empty `photos/`, and a dot-file
so `-a` has something to reveal.

- §1 *where are you:* info `pwd` → info `cd notes` then `pwd` → info `cd ..`.
- §2 *what's here:* info `ls` → info `ls -a` → info `ls -l` → info `ls -la` (short flags
  stack, one dash and both letters) → info `ls --help`.
- §3 *flag:* a hidden **folder** appears, named `linuxlab-h1dd3n` — invisible to plain
  `ls`, found with `ls -a`. The flag *is* the name, so the token shows up in the listing
  itself.

**Decisions.** The hidden folder is planted when the hunt starts, not by the scaffold: the
flag is the folder's *name*, so a pre-scaffolded `.linuxlab-h1dd3n` would be visible the
moment the student runs `ls -a` in §2 — findable before the hunt. Planting it at the hunt
also means plain `ls` never shows it. This is also why L1's flag card is the one place a
flag card has nothing to withhold: there is no command to name but `ls -a`, which the
level has already taught.

**The `ls --help` card does not send anyone hunting through the output.** It used to
hint that the `-a` and `-l` they had just used "are listed there too", which is a page of
dense noise, much of it OSC-8 hyperlink escapes — a terminal without hyperlink support
prints those inline, so the first flag a student needs arrives garbled as `-a-a, --allo`.
The card now says the manual *is* there when they need one flag, that nobody reads one
cold, and tells them to scroll away.

### L2 — files · `cat mkdir touch mv cp rm rm -r` (flag: in a note)

Scaffold `notes/todo.txt` + `notes/ideas.txt` (the token, `linuxlab-n0t3m4n`).

- §1 *read first:* info `ls` (look before you open anything) → info `cat notes/todo.txt`
  → task `mkdir done` → task `touch first.txt` → task `mv first.txt done/`.
- §2 *keep a copy:* task `cp notes/todo.txt done/` → task `mv done/todo.txt backup.txt`.
- §3 *no trash can:* info `ls` + `ls done` → task `touch throwaway.txt` → task
  `rm throwaway.txt` → task `rm -r done` (a folder and its contents: plain `rm` refuses,
  which is the lesson the hint points at).
- §4 *flag:* one of the notes remembers the magic word — read it with `cat`.

**Decisions.** The `mkdir done` hint is **`ls -l`, not plain `ls`**: a bare listing prints
the name `done` among the files with nothing marking it as a *folder*, so a student who
ran `ls` would have passed the card with no way to tell — the one thing a hint must never
do. The `d` in the first column answers it. §3 opens on the regroup beat (`ls` + `ls done`)
because this was eight graded cards with nothing to regroup at, which is where freshmen
stall; the folder is full and the next three cards delete, and *looking* is the habit that
section is about, so looking is the card.

### L3 — text-io · `echo > >> < | tee bc` (flag: a folder that lands)

The constructs, on a playground deliberately **flat and small** — two short to-do lists, a
sheet of arithmetic and a note, no folders. That is L4's whole subject, and it is why L3
goes first rather than after. Scaffold `today.txt` and `later.txt` (both a few lines —
§1's second task grades their sum), `problems.txt` (a handful of expressions, answered
one per line) and `about.txt`.

- §1 *say a line:* info `echo "hello"` → task save it into `greeting.txt` → task both
  lists into `both.txt`, which is where `>` is generalised: it catches whatever a command
  printed.
- §2 *add to it, don't wipe it:* task append a second line without losing the first → a
  text-only beat spelling out `>` replaces / `>>` adds.
- §3 *hand a file in:* info `bc < problems.txt` (four answers and you typed no sum at
  all) → task `bc < problems.txt > answers.txt`, **both constructs in one command**.
- §4 *see it *and* save it:* info `echo "2 + 3" | bc` → task `echo "all done" | tee
  done.txt` (1 line, and it is on screen too).
- §5 *flag:* an `inbox/` **appears** — a folder and two files that were not there a
  moment ago — and the word is inside one of them. Token `linuxlab-r3d1r`.

**Decisions.**

- **`echo` opens the level, not `wc`.** `wc` was a strange place to start text i/o:
  `wc -l` is a counting tool, and the construct being taught was where output *goes*.
  `echo` is the commonest way to make a line of text and it makes `>` demonstrable with
  nothing else in the level, so `wc` moved to level 4. This ordering is also the only one
  in which L3 stays small enough for its `<` beat to have something to feed.
- **The level borrows exactly one program, `bc`, and only to make `<` and `|` mean
  something.** With `echo` and `cat` alone, `cat f` and `cat < f` print **identical
  bytes** (checked with `md5sum`, not assumed), so the `<` beat could only say "look
  where the file name sat", which is the least motivating thing in the level. A program
  fixes it: a program *reads its input* rather than naming a file, which is the whole
  reason `<` exists, and the shape every program in a programming problem has when the
  judge hands you the input as a file. `bc` is the vehicle because it is a calculator —
  `bc < problems.txt` returns the four answers on the sheet and `echo "2 + 3" | bc`
  returns one, so both constructs arrive with a real result instead of a resemblance.
  `tr` would have proven the stronger point (`tr: extra operand`, because `tr` has no
  file operand at all) but that is a second tool to carry, and `bc` is the one a beginner
  can read.
  - **The honesty constraint:** GNU `bc` *does* accept a file operand, so
    `bc problems.txt` prints the same four answers and there is no contrast to show. The
    cards therefore never claim `<` changes the output — they claim what `<` is *for*.
- **`tee` is introduced *with* the pipe.** It had been put in L4 on the reasoning that it
  is a tool that consumes a construct, but its whole lesson is one visible difference —
  `>` leaves the screen empty and `tee` does not — and `echo "all done" | tee done.txt`
  shows that with no filter tool at all, right after the `|` that creates the slot `tee`
  occupies.
- **§3's section lead says "so far every command has said which file it wants"**, not
  "every command so far named its file": L1's `ls -a` and `ls -l` and L2's
  `cat notes/todo.txt` name files too, and the second phrasing was arguing against its
  own level.

### L4 — read-it · `cat wc -l wc -w grep grep -c grep -l grep -R find sort uniq` (flag: a year that appears)

L3's extension, and the only level with a tree: these are the commands that *read* text,
which is what makes the pipe worth having, and every one of them earns its keep at the end
of a pipeline — which is how ten commands fit one level without ten levels' worth of
beats. `cat` opens the level as a revisit on purpose: it is the one tool the student has
met twice already, and seeing it here with a job to do makes the shape of the level
obvious before the unfamiliar ones arrive. Scaffold `today.log` (10 lines, whose exact
counts are graded — keep them stable), `urls.txt`, `about.txt` (no 404s), and a
`logs/` tree with both quiet years and three files carrying 404s, so `-l` has something to
list and something to leave out.

- §1 *count first:* info `cat today.log | wc -l` → info `cat today.log | wc -w` (`-l`/`-w`
  are a choice, not part of the name) → task both of this year's logs into
  `lines-2026.txt`. `>` is old news by now.
- §2 *find the lines that matter:* info `grep 404 today.log` → info `grep -c 404
  today.log` (the whole output shrinks to one number) → task the visitor requests into
  `my-visitors.txt`, piped through `tee` — a callback to L3.
- §3 *sort, then uniq:* info `sort urls.txt` → info `uniq urls.txt`, **which visibly does
  nothing** → task `sort urls.txt | uniq > distinct.txt`. The `sort`/`uniq` pair earns its
  section because the order is the lesson.
- §4 *every file, not just this one:* info `find . -name "*.log"` → task them into
  `log-files.txt` (i.e. it walked into `archive/`) → info `grep -R 404 .` → task
  `grep -R -l 404 . > with-404.txt` (a list is what a pipe is for).
- §5 *flag:* the tree grows — `logs/2024/` and `logs/2024/archive/`, two more logs, one
  filed deeper than the other — and the word is inside the deeper one. Token
  `linuxlab-s33rch`.

**Decisions.**

- **The `urls.txt` fixture is built to keep §3's beat surprising:** eight lines, five
  distinct paths, and **no two repeats sitting next to each other**. Tidy it into a block
  per path and `uniq` alone starts working, so the task stops needing the pipe and the
  lesson goes with it. Nothing enforces that shape, so this sentence is the thing that has
  to survive an edit to the fixture.
- **The dates inside the two `logs/2024/` logs are 2024**, matching the folder they sit
  in. They were 2025 at first, which would have taught the exact opposite of this level's
  subject on the one tree that carries the flag — nothing asserts it, so it is the
  fixture's job to stay honest.
- `find` and `grep -R` are the two commands here the tour does *not* name — `find`
  because a single file needs no finding, `-R` because the tour's grep has one file in it.
  They earn their place by having a tree to work on.

**Known outlier, left alone on purpose:** fourteen cards and ten commands in one level.
Splitting it into a seventh level would renumber the deck's `#gamelevel` labels, so that is
a decision about the deck, not a cleanup.

### L5 — executables · `chmod +x ./ PATH` (flag: a script you have to run)

**The script is planted, not written.** L5 used to open with
`echo "echo hello!" > hello`, which asks for the one thing the level had never taught —
what a script file *is* — before saying anything about running it. So `hello` is in the
scaffold, already holding two lines (a `#!` shebang and the `echo`), mode 0644 so `chmod +x`
is real work, and the level is the walk from "what is in this file" to "that file is a
command".

- §1 *what is inside it:* info `cat hello` → info `ls -l hello` (the mode ends in `-`) →
  task `chmod +x hello` → info `ls -l`, the **whole folder**, because the note asks for a
  comparison with `about.txt` and a one-file listing cannot show both. (This card used to
  command `ls -l hello` while asking for exactly that comparison — a card asking for
  something its command cannot show. Found by *watching the reorder land*, not by a test:
  nothing structural was wrong with it.)
- §2 *why not just `hello`?* info `hello` (command not found) → info `echo $PATH` (your
  folder is not on `PATH`, which is exactly why the bare name failed).
- §3 *run it:* task `./hello > out.txt`.
- §4 *flag:* a script has appeared among three decoys, and reading it gives nothing away.
  Find it with `grep -Rl bash .`, read it, then run it. Token `linuxlab-m4k3r5`.

**§2 and §3 are in that order deliberately.** They used to be the other way round, which
put the ✅ for `./hello` *before* the explanation of why the bare name fails — the level
asked a question the student had already stopped having, and the answer read as trivia. The
failure has to come first, so the level runs chmod → it still will not run by name → why →
run it, and the payoff lands last. **§1's `chmod +x` has to stay before §2 too**, or the
bare-name failure is only a *permission* failure: bash would say `permission denied`
instead of `command not found` and the card would be quietly lying. No test catches
either of these — they are content lies, not structural breaks — which is why they are
written down here.

**The flag is assembled, not planted.** The hunt used to ask the student to *make* their
own tool say the word in `message.txt`, which nobody would ever do: `cat message.txt` is
shorter, needs nothing taught, and was what the card's own last hint handed over. The word
now arrives inside a script the student did not write, and the interesting part is what
that script does **not** contain:

- **Nothing on disk holds the word.** Not the token, not either half, not any fragment —
  so the screen scanner, which wants the whole token, stays quiet until the script runs.
- **No encoding to spot.** Two were tried and rejected. *Two halves on adjacent lines*
  (`first=linuxlab` / `second=m4k3r5`) are joinable by eye, so a student could type the
  word without running anything. *Octal escapes* (`printf '\154\151\156'`) put the answer
  in one recognisable line among a hundred, which is the same tell — and `printf '\154'`
  undoes it in one command. Instead the characters are **derived from the report's own
  work**: `huntScript` is a hundred-odd lines of plausible nightly-report generation around
  one helper, `glyph`, that reduces a number modulo 37 into a 37-character alphabet.
- **The call shape is camouflage.** `glyph` is called dozens of times, and twenty-odd
  of those calls spell the ordinary words the report prints — `sweep`, `rotate`,
  `verify`, `keep` — so a reader's eye has already learned to skip them.
- **The word is a by-product.** Each log row carries a numeric `code` column of the kind
  any checksum-bearing log has; the row's status letter is `glyph "$code"` and the same
  value accumulates into `sig`, which prints as the report's last field like any other
  summary. Every number sits in one narrow band, so no single one is any more meaningful
  than the next.
- **Nothing spells it out for the reader.** The only reward for reading carefully is that
  the status column spells the word top to bottom — which still takes a *running* script to
  see. `sh start` works as well as `chmod +x` and `./start`, and `start` is planted 0644
  so `chmod +x` is re-taught on the payoff.

Decoys `scratch.txt`, `cleanup.log` and `inventory.csv` are planted alongside, none of
which mentions `bash`, so `grep -Rl bash .` returns exactly two files — the `hello` the
level already taught and `start`. That is why hint 2 says "them".

**One label, twice.** The report printed `stages` in both its header and its footer, far
enough apart on the page that no reader connects them, with different values under each:
the header spelled the three stage names out
and the footer counted them. In the one level whose premise is that this output looks like
a real nightly job, a student who trusts the header and then reads the footer concludes the
script is lying to them. The footer says **`by stage`** now. Nothing tested this — both
values were correct — so like the ordering above it is written down rather than checked.

### L6 — permissions · `chmod u/g/o ± rwx · chmod 600/640/700 · ls -ld · id` (flag: the vault)

**The letters come first.** `chmod g+r` says *who* and *what* in the same breath, and a
student who has that can read `-rw-r--r--` without being told; the numbers are then just
the letters added up, introduced as the short form rather than as the real thing. The deck
already leads its `chmod` slide this way and the level used to contradict it by opening on
`chmod 600`. Both forms and both signs are used: `u-`, `a+`, `go-`, then 600 and 640.
Scaffold `data.txt` (0644), `public.txt` (**0600**, so "let everyone read it" is real
work), `notes.txt` (**0666**, so `go-w` has two columns to move), and `secret/` holding
`flag.txt` — the vault's own mode is load-bearing, see below.

- §1 *read the mode:* info `ls -l` → a text-only beat spelling the nine letters out, so
  the column is decodable before anything changes it.
- §2 *move one letter at a time:* task `chmod u-w data.txt` (**0444** — only the first
  group moves) → task `chmod a+r public.txt` (**0644**; `a` is the three groups at once) →
  task `chmod go-w notes.txt` (**0644** — `go` is `g` *and* `o`, so the shorthand is *used*,
  not just described) → a text-only beat giving each letter its number.
- §3 *and when you know the letters:* task `chmod 600 data.txt` → task `chmod 640
  notes.txt` (one digit apart in the group column is the whole difference between the two)
  → info `ls -l` to compare them side by side.
- §4 *the vault* — **four cards, surprise first**, because the whole lesson is a
  distinction the student has to *watch* rather than be told: info `ls secret` (**it
  works** — `r` on a folder is reading the *names* inside, and nothing more) → info
  `cat secret/flag.txt` (**Permission denied** — you can see the name and you cannot have
  the file, which is the second permission, on the same folder) → info `ls -ld secret`,
  where `drw-------` finally shows the missing letter and spells the rule: `r` reads the
  names, `x` is what lets you *go in* → task `chmod 700 secret`.
- §5 *you, and your groups:* info `id` (read as three fields: `uid=`, `gid=`, `groups=`) →
  a text-only beat defining a **group** → a text-only beat on **root**: user number 0, not
  bound by the permission bits at all, so the student's ordinary `uid` is what makes §4's
  `chmod 700` mean something.
- §6 *the vault opens:* the folder you just opened is yours alone — `cd secret`, `cat
  flag.txt`. Token `linuxlab-k3y5`. The card ties it back: the `x` you were missing in §4
  is exactly what lets the `cd` and the `cat` work now.

**The vault is a real lock, not a gesture.** `secret/` was seeded **0755**, and at 0755 the
flag was readable *before* the level's `chmod` and after it — the climax was built on a
state change the player could not observe, or believe. At **0600** the semantics are
exactly the ones the level is trying to teach, verified by hand: a folder with no `x`
withholds from its **owner** too, so `cd secret` is refused and `cat secret/flag.txt` says
`Permission denied`, and `chmod 700 secret` is what turns both on. `ls secret` works at
0400 and 0600 — `r` is reading the names — which is precisely the split §4 exists to show.
That is also why §4 is four cards and not two: the denial has to happen *before* the
diagnosis, on the student's screen.

The task's hint ladder offers `chmod 700 secret` and `chmod u+x secret` and says outright
that plain **`chmod +x` is wrong** — on a 0600 folder it yields **0711**, handing the right
to go in to group and other as well. Verified, not assumed; the climax is no place to
discover that the obvious command misses the check.

Two consequences worth not undoing:

- The seed plants **`secret/flag.txt` before the `secret/` directory itself**, because
  `putMode` writes a file *through* its parent and a 0600 folder has no `x` to write
  through. The order is load-bearing and a test holds it.
- The sealed folder is invisible to two things a test does — `WalkDir` cannot descend and
  `t.TempDir`'s cleanup cannot delete the tree — so `unseal` opens such folders where a
  test needs to see inside, **after** any mode assertion has already read what it wanted.
  Never put `unseal` inside the scaffold path: opening the vault to make a test convenient
  satisfies `chmod 700 secret` before the student arrives.

**`g` means the file's group, not "your group".** §1 had been saying `g` is *your* group,
which is true in the playground only because the player made every file in it and
inherited their own primary group. The deck's own example is `-rw-r--r-- 1 you staff …` — a
*different* group from the owner. Both §1's text beat and §2's `go-w` ✅ line now say "the
group that owns it", and §1 points forward to §5 rather than leaving the word undefined.

**§5 defines "group" because the deck uses the word without defining it**: the permissions
slide labels the fourth column *group* and its own fence reads `-rw-r--r-- 1 you staff …`
— owner and group are different people there. The game, as the only complete reference a
student who missed the session has, is where the word gets defined, and it is checkable
rather than abstract: both names are on the student's screen at the same moment. **No deck
change** — the slide has no headroom and the game's definition is additive, not corrective.

### ✦ habits — the toolbox (`num 0`, `extra`, no token)

Menu-only: never part of the 1→6 sequence, `-level` cannot reach it, and finishing it
returns to the menu and ticks its own ✦ row. Six info cards, then the Ctrl-D lesson:

- §1 *the manual:* info `man ls` (read a little, `q` to quit), info `ls --help`.
- §2 *memory:* info `cd sec` finished with **Tab**, info `ls` then **↑** to recall it, info
  `clear` (the rows go; ↑ still has the history).
- §3 *the keys:* info `sleep 60` then **Ctrl-C**, then the `exitKey` beat: the game reads
  **Ctrl-D** itself and asks `y to leave for real · anything else stays` before handing the
  key to bash. No flag — the payoff is the habits.

**Ctrl-D inside the toolbox is read by the game, not passed to bash** (`exitKey` steps +
`keyboard.setCapture`). The key that ends a shell is exactly the habit the card teaches, so
the game asks first: Ctrl-D prints the question, `y` leaves for real and anything else
stays and finishes the card. The protocol is a pure function (`trapKey`) so it is testable
without a terminal, and the capture is armed only for that beat — dropped on the way out,
so bash gets the keyboard back immediately.

## 8. Build status — built & tested

Built: all six levels and the toolbox, played through end to end on Linux in a pty —
cards, hints on the second miss, the time-based nudge, the escape line, flag hunts, the
level breaks, `Ctrl-D`, the exit ramp, and the toolbox's own Ctrl-D trap. It was also
played in a narrow (60-column) pty, since the cards lay themselves out to the window you
are in. The build, deploy and dev-tool commands are in `README.md`; the decisions behind
what was built are in §1 and §7.

### The suite is structure-only, on purpose

**No test looks at what a level says.** `levels_test.go` holds the rules the game depends
on, checked against every step of every level, so a level added tomorrow is covered without
anyone remembering:

| invariant | test |
|---|---|
| no step is already done when the student arrives (no free steps) | `TestNoTaskIsFree` |
| a token never appears in a goal / note / hint / ✅ line | `TestTokensStayOutOfTheProse` |
| every step has hints, a ✅ line, and a check or a token | `TestEveryStepIsComplete` |
| every hunt's word is on the disk after its plant — or, where the level says `assembled`, deliberately not | `TestFlagsArePlanted` |
| every numbered level ends in exactly one hunt, and no two share a token | `TestEveryLevelEndsInItsOwnHunt` |
| the numbered levels are 1..N; the toolbox is last, `num 0`, extra, never "the last level" | `TestLevelNumbers` |
| a file is never planted inside a folder that was already planted without owner `x` | `TestAPlantInsideASealedFolderComesFirst` |

**Why the fixture counts are gone.** They used to sit here — `today.txt` has three lines,
`urls.txt` has five distinct paths with no adjacent repeats, the hunt's script is fifty
lines and calls its helper twenty times — and they only recorded what one author decided
on one day, then failed the next person who reworded a card or renamed a helper. The worst
was the last: renaming the hunt's helper from `glyph` to `mark`, leaving the level's
behaviour byte-identical, failed a test asserting "has 20 glyph calls" while the replay
beside it played level 5 to `LEVEL 5 COMPLETE`. That test is named here so nobody
reinstates it.

**Content quality is review, not test.** That a hunt's script is hard to read rather than
merely unreadable, that a hint ladder escalates, that a goal names its target — those are
judgement calls, recorded in §1 and §7, and a test can only ever hold them against the
*next* person's rewording. Two layers do the work instead, and both are in the suite:

- **`replay_test.go` plays each level**, typing real commands into a throwaway playground
  through the real binary on a real pty, and waits for the level's own completion banner.
  It is the only check that exercises content, artifact checks and layout together, and
  the only one that would notice a card whose goal has stopped matching what satisfies it.
  Two rules make it trustworthy rather than decorative. Each command is followed by a wait
  for the *echo* of that command, counted from a mark taken before it was typed, and then
  for the prompt: waiting for "the prompt" alone is a race, because a card draws its own
  prompt before the student types and the shell draws another when it is done, so a naive
  replay satisfies the second wait with the first prompt and fires the next line into a
  shell still busy with this one. And `expectCard` reads the whole screen, so a card an
  earlier step left there can answer for a later one — `TestEveryStepHasAMoveInItsOwnReplay`
  requires one move per step, in order, each naming its own step's goal, which closes that.
  On its first run it found three real things, all of them content: level 1's flag is in a
  **dot folder's** name, so the hunt needs `ls -a` and the replay's plain `ls` hung exactly
  as a student would; level 2's word was not in the file the replay guessed; and level 5
  still told the student to compare a mode with a file a redesign had deleted. The dev tool
  and the test are one thing — see `README.md`.
- **`zz_dump_test.go` (`DUMP=1`) prints** the content to be read rather than asserted.

**`bc` has no test of its own**, because it is a runtime dependency: what it answers is
`bc`'s business, not the game's, and the replay runs the real thing anyway — so the level 3
subtest *skips* where `bc` is absent rather than failing a suite that has nothing to say
about it.

### The layout, on the buffer

Every screen is rendered at 40–200 columns with colour on and off, and the width of each
line is measured *by the glyphs* (an emoji is two columns wide, an escape sequence is
none):

| invariant | test |
|---|---|
| no line overruns the window, except a token nothing can break (a URL, a long path) | `TestNothingOverrunsTheTerminal` |
| a wrap never cuts an escape sequence in half | `TestNoEscapeIsEverCut` |
| at 80 columns the look is the one §5 draws (the bars, the ≤ 8-line card) | `TestAStandardTerminalKeepsTheLookTheDesignDraws` |
| a wrap is greedy, breaks only on spaces, and hands back untouched slices | `TestWrapIsGreedyAndVerbatim` |
| a wrapped hint keeps its voice — the command stays bold cyan across the break | `TestAWrappedHintKeepsItsVoice` |
| a `` `command` `` moves down whole and stays styled; a flag stays with its command | `TestCodeSpansStayWholeAndStyled` |
| a glob inside backticks keeps its star | `TestMarkupKeepsAGlobIntact` |
| a hanging indent is as wide as the prefix it hangs from | `TestHangAlignsUnderItsPrefix` |
| every line begins at column 0 on a terminal — no staircase down the right | `TestTheStudentSeesNoStaircase` |
| a scripted run stays plain text: no colour, no carriage return | `TestAScriptedRunStaysClean` |

### The screen, not the bytes

A buffer is one layer short of the truth: raw mode, the shell and the relay only exist in a
real session, and that is exactly where a bug lived that every byte-level check passed. The
game was writing bare LFs to a raw-mode terminal, where the keyboard's raw mode has taken
the kernel's newline translation away from the game's own output, so on the real screen
every card stair-stepped to the right while its bytes stayed perfect. So the layer that
was missing is now a test: it builds the real binary, runs it on a pty of a pinned width,
drives it by waiting for the game's own `tuxland$ ` prompt — the game draws that when it
is ready for the next command, so the handshake is a fact rather than a guess at how long a
shell takes — and then renders the capture through a model of the terminal and reads the
screen.

| invariant | test |
|---|---|
| every row of a real session begins at column 0, at any width | `TestTheStudentSeesTheSession` |
| the card, the note, the hint, the ✅ and the bye each keep their indent on screen | `TestTheStudentSeesTheSession` |
| a hint that wraps hangs under its own prefix, not under the line's length | `TestTheStudentSeesTheSession` |
| a command never loses its word to a line break, on the way out either | `TestTheStudentSeesTheSession` |
| the game speaks **after** the shell, even for a slow command | `TestTheGameWaitsForBashToFinish` |
| Ctrl-D is asked about before it ends the shell; `y` leaves, anything else stays | `TestTheExtraTrapsTheExitKey` |
| the toolbox ticks its ✦ row on the menu it returns to | `TestTheExtraTrapsTheExitKey` |

A full terminal emulator was considered and not built: the game emits seven escape
sequences in total and never leaves the main screen, so a faithful VT model would be a large
program emulating far more than this game does. What catches this class of bug is a model
of the part that decides where a line lands — CR, LF, the right edge — and that model is
shared with the layout tests above.

### The menu and the progress file

| invariant | test |
|---|---|
| the keys move the selection — arrows (incl. split across reads), digits, Ctrl-D/C | `TestMenuKeysMoveTheSelection`, `TestAnArrowKeySplitAcrossReadsStillMovesOnce` |
| a digit above 6 is ignored — the toolbox has no number | `TestMenuKeysMoveTheSelection` |
| a repaint replaces the line, never adds a second one or leaves a tail | `TestARepaintReplacesTheLineItself` (model) + `expectMenu` (pty) |
| the level name survives the colour (measured stripped, not with the escapes) | `TestTheMenuNamesTheLevelInColour` |
| the menu's committed line is the level that opens, at any width | `TestTheMenuPicksTheLevel` (9 choices on a real pty) |
| finished levels carry a ✅ and only those — numbers still lined up — and the cursor starts on the first one left | `TestTheMenuRemembersWhatYouFinished` (5 seeded states) |
| the toolbox is remembered apart (never counted, never "next up"), and a junk `extra` flag is ignored | `TestTheExtraIsRememberedButNotCounted`, `TestAHandEditedExtraFlagIsIgnoredWhenItIsNotTrue` |
| the file is the shape described, read leniently, written atomically and sorted | `TestBadProgressIsIgnoredNotFatal`, `TestTheProgressFileIsNeverLeftHalfWritten` |
| progress survives between runs, is written the moment a level is finished, and lands in a lab root that does not exist yet | `TestFinishingALevelWritesTheFile`, `TestProgressIsRememberedBetweenRuns`, `TestProgressIsSavedIntoALabRootThatDoesNotExistYet`, `TestNextOffersTheFirstUnfinishedLevel` |

### The one habit worth naming

**A layout invariant about what the student *sees* needs a model of the terminal, not a
check on the bytes** — and a fixed wait for an event you can be *told* about is a wait that
is either slow for everyone or wrong for someone. Both bugs above were one of those two
mistakes wearing different clothes, and both were invisible to a suite that was measuring
the right thing one layer below the truth.

## 9. Explicit non-goals for v1

- No command-line parsing / in-game keywords (`next`, `hint`, `help`).
- No quizzes beyond the flag token, and no simulated shell.
- No hosted wargame server, and no cross-platform portability work.
- No sudo and no system changes; nothing outside the scratch dir may be touched.
- No timing tuning: the game answers on bash's prompt (§3), so there is nothing to tune.

## 10. Known hazards, accepted

- A nudge or ✅ printed while the student is mid-line leaves their text above it. Nudges are
  rare and the check only fires after a submitted line, so this is left alone.
- L4 is long (§7). Accepted rather than split, because splitting renumbers the deck.