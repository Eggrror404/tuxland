// cli.go is the game's entry-point API: the handful of exported calls the thin
// root `main` package needs. Everything else stays inside the package — there is
// no reason for a program to reach in.
package tuxland

import (
	"fmt"
	"io"
)

// NumLevels is how many numbered levels the game has, for flag validation in
// main and for the menu's digit-jump. The habits toolbox is extra — it has no
// number, and `-level` cannot reach it.
func NumLevels() int { return len(numberedLevels()) }

// List prints the level table plus the play instructions, and stops.
func List(w io.Writer) { newUI(w).list(loadProgress()) }

// Play is a full run starting at start (0 = the menu, N = straight into level N).
func Play(w io.Writer, start int) { play(newUI(w), start) }

// Usage prints the command summary — main hands it to flag.Usage, so flag's
// "try -h" and a bad -level both end up here.
func Usage(w io.Writer) {
	fmt.Fprint(w, `tuxland — the guided command-line game

  tuxland            the menu: pick a level, then play it
  tuxland -level 3   skip the menu and start at level 3 (also the rehearsal path)
  tuxland -list      list the levels and exit

Level 3 needs `+"`bc`"+` (a calculator); the game says so and stops if it is
missing. The ✦ habits toolbox wants `+"`man`"+`, but does without it.

The menu also carries ✦ habits — the toolbox: man, Tab, ↑ and the keys that
get you out. It has no number and no flag; it is picked from the menu.

Everything happens in ~/linux-lab/level-NN-name (or $LINUXLAB_ROOT). The
levels you finish are remembered in ~/linux-lab/progress.json, so the menu
offers you the next one; delete that file to start over.
You type real bash; the game watches the filesystem and says so when a step
is done. Ctrl-D or "exit" leaves.
`)
}
