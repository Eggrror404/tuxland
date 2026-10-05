package tuxland

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestDumpLevels is a review aid, not an assertion: run with
//
//	DUMP=1 go test ./internal/tuxland -run TestDumpLevels -v
//
// and it prints every level, its identity fields, and every card in order — the
// fastest way to read a content change end to end without a pty.
func TestDumpLevels(t *testing.T) {
	if os.Getenv("DUMP") == "" {
		t.Skip("set DUMP=1 to print the level dump")
	}
	for _, lv := range levels {
		num := fmt.Sprintf("%d", lv.num)
		if lv.extra {
			num = "✦"
		}
		fmt.Printf("\n%s\n", strings.Repeat("━", 78))
		fmt.Printf("%s  LEVEL %s · %s\n", strings.Repeat("━", 78), num, lv.title)
		fmt.Printf("name=%q  emoji=%s  tagline=%q\n", lv.name, lv.emoji, lv.tagline)
		fmt.Printf("teaches: %s\n", strings.Join(lv.cmds, " "))
		fmt.Printf("uses:    %s\n", strings.Join(lv.used, " "))
		fmt.Printf("lead:    %s\n", lv.lead)
		fmt.Printf("take away: %s\n", lv.learned)
		fmt.Printf("try this:  %s\n", lv.extraTip)
		fmt.Printf("%s\n", strings.Repeat("━", 78))

		for si, sec := range lv.sections {
			fmt.Printf("\n  §%d · %s\n", si+1, sec.title)
			if sec.lead != "" {
				fmt.Printf("      %s\n", sec.lead)
			}
			for _, st := range sec.steps {
				kind := map[kind]string{
					kindInfo: "INFO", kindTask: "TASK", kindFlag: "FLAG",
				}[st.kind]
				if st.noCmd {
					kind = "TEXT "
				}
				fmt.Printf("\n      [%s] %s\n", kind, st.goal)
				if st.note != "" {
					fmt.Printf("             note: %s\n", st.note)
				}
				for i, h := range st.hints {
					fmt.Printf("             hint %d: %s\n", i+1, h)
				}
				if st.done != "" {
					fmt.Printf("             done: %s\n", st.done)
				}
				if st.token != "" {
					fmt.Printf("             token: %s\n", st.token)
				}
				if len(st.plant) > 0 {
					fmt.Printf("             plants %d file(s) on activation\n", len(st.plant))
					for _, p := range st.plant {
						kindp := "file"
						if p.dir {
							kindp = "dir "
						}
						fmt.Printf("               - %s %s\n", kindp, p.at)
					}
				}
			}
		}
	}
	fmt.Printf("\n%s  numbered: %d, toolbox extra: %v\n", strings.Repeat("━", 78),
		NumLevels(), extraLevel().extra)
}
