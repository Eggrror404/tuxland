// Command tuxland is a guided command-line game: real bash, real commands, real
// files, and a card at a time telling you what to make happen.
//
// The game hands the player a real bash on the real filesystem and supervises
// it: a card says what to do, the artifact checks say whether it happened, hints
// arrive by themselves, and each level ends with a flag hunt. The game never
// parses what is typed and never touches anything outside ~/linux-lab.
//
// See internal/tuxland for the game itself, DESIGN.md for the design and
// README.md for how to play.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tuxland "github.com/Eggrror404/tuxland/internal/tuxland"
)

func main() {
	level := flag.Int("level", 0, fmt.Sprintf("start straight at level N (1-%d)", tuxland.NumLevels()))
	list := flag.Bool("list", false, "list the levels and exit")
	flag.Usage = func() { tuxland.Usage(flag.CommandLine.Output()) }
	flag.Parse()

	// Whatever happens, the student's terminal is handed back in a usable state.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-ch
		tuxland.RestoreRaw()
		os.Exit(1)
	}()

	switch {
	case *list:
		tuxland.List(os.Stdout)
	case *level < 0 || *level > tuxland.NumLevels():
		fmt.Fprintf(os.Stderr, "tuxland: -level must be 1..%d\n\n", tuxland.NumLevels())
		tuxland.Usage(os.Stderr)
		os.Exit(2)
	default:
		tuxland.Play(os.Stdout, *level)
	}
}
