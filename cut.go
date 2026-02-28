package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

type Cut struct {
	Start string
	End   string
}

func parseStamps(stamps string) []Cut {
	var cuts []Cut
	for cut := range strings.SplitSeq(strings.TrimRight(stamps, ","), ",") {
		cut = strings.TrimSpace(cut)
		if cut == "" {
			continue
		}
		cutArgs := strings.SplitN(cut, "-", 2)
		if len(cutArgs) != 2 {
			log.Fatalf("unexpected timestamp format: %q", cut)
		}
		cuts = append(cuts, Cut{
			Start: strings.TrimSpace(cutArgs[0]),
			End:   strings.TrimSpace(cutArgs[1]),
		})
	}
	return cuts
}

func applyCuts(cuts []Cut, inputFile string) {
	for i, c := range cuts {
		outputPath := fmt.Sprintf("cuts/cut_%d.mp4", i)
		cmd := exec.Command(
			"ffmpeg",
			"-ss", c.Start,
			"-to", c.End,
			"-i", inputFile,
			"-c", "copy",
			outputPath,
		)
		log.Println(string(cmd.String()))
		if err := cmd.Run(); err != nil {
			log.Fatalln("cut", i, "failed:", err)
		}
	}
}
