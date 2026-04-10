package cli

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
)

func BuildReviewJob(args []string) (ReviewJob, error) {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})

	var jsonOutput bool
	var noVisual bool
	var failBelow string

	fs.BoolVar(&jsonOutput, "json", false, "")
	fs.BoolVar(&noVisual, "no-visual", false, "")
	fs.StringVar(&failBelow, "fail-below", "", "")

	if err := fs.Parse(normalizeFlagArgs(args)); err != nil {
		return ReviewJob{}, err
	}
	positionals := fs.Args()
	if len(positionals) == 0 {
		return ReviewJob{}, errors.New("document type is required")
	}
	if strings.ToLower(strings.TrimSpace(positionals[0])) != "pptx" {
		return ReviewJob{}, fmt.Errorf("review currently supports only pptx")
	}
	if len(positionals) < 2 || strings.TrimSpace(positionals[1]) == "" {
		return ReviewJob{}, errors.New("file is required")
	}
	threshold := 0
	if strings.TrimSpace(failBelow) != "" {
		value, err := strconv.Atoi(strings.TrimSpace(failBelow))
		if err != nil || value < 0 || value > 100 {
			return ReviewJob{}, fmt.Errorf("invalid --fail-below: %s", failBelow)
		}
		threshold = value
	}
	return ReviewJob{
		DocumentType: "pptx",
		FilePath:     strings.TrimSpace(positionals[1]),
		EnableVisual: !noVisual,
		FailBelow:    threshold,
		JSONOutput:   jsonOutput,
	}, nil
}
