package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

type Cut struct {
	Start string
	End   string
}

func parseStamps(stamps string) []Cut {
	fmt.Println(stamps)
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

func applyCuts(cuts []Cut, inputFile, outputFile string) {
	listFile, err := os.Create("cut_files.txt")
	if err != nil {
		log.Fatalln("failed to create list file:", err)
	}
	defer func() {
		listFile.Close()
		os.Remove(listFile.Name())
	}()

	if err := os.Mkdir(".cuts", 0755); err != nil && !os.IsExist(err) {
		log.Fatalln("failed to create .cuts directory:", err)
	}
	defer os.RemoveAll(".cuts/")

	for i, c := range cuts {
		outputPath := fmt.Sprintf(".cuts/cut_%d.mp4", i)
		cmd := exec.Command(
			"ffmpeg",
			"-ss", c.Start,
			"-to", c.End,
			"-i", inputFile,
			"-c", "copy",
			outputPath,
		)
		if err := cmd.Run(); err != nil {
			log.Fatalln("cut", i, "failed:", err)
		}

		if _, err := fmt.Fprintf(listFile, "file '%s'\n", outputPath); err != nil {
			log.Fatalln("failed writing to list file:", err)
		}
	}

	cmd := exec.Command(
		"ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile.Name(),
		"-c", "copy",
		outputFile,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("ffmpeg concat failed: %v – output: %s", err, output)
	}
}
