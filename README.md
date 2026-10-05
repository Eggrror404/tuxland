# tuxland — the guided command-line game

A hand-held practice field for learning the **Linux command line**. You type **real
Linux commands** in **real bash**; the game checks what you produced (real files, real
permissions) and hints when you're off. Training wheels, not a CTF.

The real practice fields — **OverTheWire Bandit** and **pwn.college** — are where to
go when you finish here.

**Playable now** — all six levels plus the ✦ habits toolbox, built and tested (see
*Status* below).

## How it plays

You get one terminal. `tuxland` opens on a **level menu**: the six numbered levels
with the ones you've already finished ticked ✅, a one-line score ("2 of 6 levels done
· next up: level 3"), and a live choice line at the bottom. Move it with **↑/↓** or
just **type a number**, press **Enter**, and you're in. The game writes a short
**card** (the current step) directly above the prompt; everything between is bash
being bash — tab-completion, arrow keys, `Ctrl-C`, colours, all of it.

- A step is done when its **artifact appears on disk** (`first.txt` exists, `data.txt`
  is `-rw-------`, …). The game never reads what you typed.
- Try twice and a **hint** shows up on its own. The first try is left silent — on a
  task card because bash's own error message has already told you what went wrong,
  and on a flag hunt because the word is usually still there and you need to look
  rather than be told. After that the hints escalate.
- Every level ends with a **flag hunt**: a `linuxlab-…` word is hidden in the
  playground. Print it (`cat`, `grep`, a script of your own…) and the game catches it
  on screen. There is nothing to type back.
- The **level summary** at the end lists the commands you just used — that's the
  win, not a grade.
- A level you finish is **remembered**: next time the menu ticks it ✅ and offers you
  the next one, so you can quit anywhere and pick up where you left off.
- The menu's last row is **✦ habits** — an unnumbered toolbox of the small shell
  habits (`man`, `--help`, Tab, ↑, `clear`, `Ctrl-C`, `Ctrl-D`). No flag and no
  number: play it whenever.

## Where to run it

**Anywhere with a shell** — Linux, macOS, a remote ssh session, a container. The only
hard requirement is a `bash` on your `PATH`; the rest of the game runs on coreutils.
Level 3 borrows one program that is not part of a base system, **`bc`**, and the game
checks for it and says so rather than starting a level it cannot finish.

```sh
go install github.com/Eggrror404/tuxland@latest
```

or download a prebuilt binary from [**Releases**](../../releases) — one static file,
nothing to install. Either way, put it on your `PATH` and:

```sh
tuxland            # the menu: pick a level, then play it
tuxland -level 3   # skip the menu, start straight at level 3 (also how you redo one)
tuxland -list      # what the levels are, without playing
tuxland -v         # which version this is
```

**Windows:** use WSL — [install WSL](https://learn.microsoft.com/windows/wsl/install)
(Win 10/11, one `wsl --install` in an admin terminal, reboot if asked), then run the
same commands inside it.

**From source:** clone and `go build -o tuxland .` (see *For developers* below).

## Safety

Everything happens in a scratch folder inside your home:
`~/linux-lab/level-NN-name/`. **Nothing outside that folder is touched** — no sudo, no
system changes, no shell history files. Bash mistakes here cost nothing.

- **Reset a level:** just run it again. Starting a level wipes and rebuilds its
  folder, so you always get a clean playground.
- **Your score:** finished levels are remembered in `~/linux-lab/progress.json` — the
  menu ticks them ✅ and offers the next one. Delete that file to start over, or:
- **Start completely clean:** `rm -rf ~/linux-lab`.
- **Leave:** `Ctrl-D` or `exit`. The playground stays where it is.
- **Point it elsewhere:** `LINUXLAB_ROOT=/some/dir tuxland` (used by the tests).

---

## For developers

- **Build:** `go build -o tuxland .` — Go ≥ 1.24, module
  `github.com/Eggrror404/tuxland`, deps are `github.com/creack/pty`,
  `golang.org/x/term` and `golang.org/x/sys` (the last for one `TIOCGPGRP`, see
  *No dead air* below); nothing else. `go test ./...` runs the level invariants (no
  step is free, tokens never leak into a card, every flag is planted), the layout
  invariants (nothing the game draws runs past the window at 40–200 columns, and at
  80 columns the cards look like the ones in the design) and real sessions on a real
  pty, checked as a *screen* rather than as bytes — `DESIGN.md` §8 has every
  invariant and the test that locks it.
- **Deploy:** one static binary, no data files, no install step, nothing to keep in
  sync. `GOOS=linux GOARCH=amd64 go build -o tuxland .` cross-compiles for any target;
  the [release workflow](.github/workflows/release.yml) builds the lot on every tag.
- **Looking at the screen without a human:** the session test doubles as the dev tool.
  ```sh
  TUXLAND_WIDTH=40,80 TUXLAND_DUMP=1 go test -run TheStudentSees -v
  ```
  plays a level at those widths and prints the screen row by row, with a ruler. Skip it
  with `-short` when you only want the fast checks.
- **Reading a content change without a human:**
  ```sh
  DUMP=1 go test ./internal/tuxland -run TestDumpLevels -v
  ```
  prints every level and every card in order — goal, note, hint ladder, ✅ line and
  what it plants — which is the whole game as text. It skips without `DUMP`.
- **Playing a level yourself, unattended:** `TUXLAND_REPLAY=N` runs just level N,
  typing real commands at a throwaway playground and waiting for the level's own
  completion banner. It is the check that exercises content, artifact checks and
  layout together, so it is where a card that reads fine but cannot be passed shows
  up. `TUXLAND_WIDTH` and `TUXLAND_DUMP` work with it too.
- **No dead air:** the game answers your Enter when bash's prompt says it is done, not
  on a timer, and a slow command is still answered *below* its output. The one thing it
  asks the terminal instead of guessing: whether bash still owns it, which is how the
  50-second nudge knows to leave you alone while you are in a pager. `DESIGN.md` §3
  has the measurement and the earlier guesses it replaced.
- **Needs:** a `bash` on the player's `PATH` (the game spawns `bash --norc
  --noprofile -i`; it fails with a clear message if bash is missing). Beyond the
  base system: **`bc`**, because level 3 feeds a calculator to teach `<` and `|`
  and there is no way to reword that beat without a second tool. So it is a hard
  dependency of that one level and nothing else: the game looks for `bc` before
  it scaffolds anything and stops with the install command rather than stalling
  at a card that cannot be passed — `-level 3` on a machine without it prints
  that message and changes nothing. **`man`**, for one card of the unnumbered habits
  toolbox. The game does not check for it and nothing is skipped: the card is still
  drawn, and bash says `man: command not found` if you run it.
  Nothing is installed and nothing is fetched: no root, no packages, no network.
  If an ssh session dies mid-level the game exits on its own and leaves no shell
  behind.
  `$LINUXLAB_ROOT` moves the playground for tests and demos, and is checked
  before anything is wiped: `~` is expanded, and `/`, your home folder and the
  directory you started the game in are refused by name.
- **Design (implemented):** the full spec is in [`DESIGN.md`](DESIGN.md) — guided labs
  on the real box (real bash in a pty, artifact checks + a flag-hunt finale per level,
  hints on mismatch, sections per level). No simulated shell. §7 is the level content
  and the reasoning behind it; §8 is what the test suite does and does not check.
- **Smoke test without a human** — a pipe is enough to see the level open and its
  first card render, but **it cannot play a level**: the game advances on bash's
  prompt, so all the piped input arrives before the first check.
  ```sh
  LINUXLAB_ROOT=/tmp/lab-smoke sh -c "printf 'ls -l\n' | ./tuxland -level 6"
  ./tuxland -list
  ```
- **Status:** built & tested — all six levels and the ✦ habits toolbox played through on
  Linux, including the pty, hints, flag hunts, the level breaks and the exit ramp, and
  again in a narrow (60-column) pty since the cards lay themselves out to the window you
  are in. The opening menu, its ✅ progress and `progress.json` are covered the same way.
  `DESIGN.md` §8 has every invariant the suite locks, what it deliberately leaves to a
  reviewer, and the two bugs a byte-level check could not see.

## How this was written

The implementation is largely AI-assisted. The design, the level content, the teaching
order and the wording are hand-written, and [`DESIGN.md`](DESIGN.md) is where the
reasoning behind them is recorded — including the decisions that look like mistakes.

MIT licensed — see [`LICENSE`](LICENSE).
