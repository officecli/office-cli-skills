package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/officecli/officecli-internal/engine"
)

type ModifyJob struct {
	SourceFilePath string
	DocumentType   engine.DocumentType
	Prompt         string
	Language       string
	Style          string
	OutputDir      string
	Mode           string
	JSONOutput     bool
}

func BuildModifyJob(args []string) (ModifyJob, error) {
	fs := flag.NewFlagSet("modify", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})

	var prompt string
	var promptFile string
	var mode string
	var outDir string
	var jsonOutput bool

	fs.StringVar(&prompt, "prompt", "", "")
	fs.StringVar(&promptFile, "prompt-file", "", "")
	fs.StringVar(&mode, "mode", "", "")
	fs.StringVar(&outDir, "out", "", "")
	fs.BoolVar(&jsonOutput, "json", false, "")

	normalized := normalizeModifyFlagArgs(args)
	if err := fs.Parse(normalized); err != nil {
		return ModifyJob{}, err
	}
	positionals := fs.Args()
	if len(positionals) == 0 {
		return ModifyJob{}, errors.New("source file path is required: officecli modify <file> --prompt \"...\"")
	}

	sourceFile := strings.TrimSpace(positionals[0])
	if sourceFile == "" {
		return ModifyJob{}, errors.New("source file path is required")
	}

	ext := strings.ToLower(filepath.Ext(sourceFile))
	var docType engine.DocumentType
	switch ext {
	case ".pptx":
		docType = engine.DocumentTypePPTX
	case ".docx":
		docType = engine.DocumentTypeDOCX
	case ".xlsx":
		docType = engine.DocumentTypeXLSX
	default:
		return ModifyJob{}, fmt.Errorf("unsupported file type %q: modify supports .pptx, .docx, .xlsx", ext)
	}

	if strings.TrimSpace(prompt) != "" && strings.TrimSpace(promptFile) != "" {
		return ModifyJob{}, errors.New("use only one of --prompt and --prompt-file")
	}
	finalPrompt := strings.TrimSpace(prompt)
	if finalPrompt == "" && strings.TrimSpace(promptFile) != "" {
		data, err := os.ReadFile(promptFile)
		if err != nil {
			return ModifyJob{}, fmt.Errorf("read prompt file: %w", err)
		}
		finalPrompt = strings.TrimSpace(string(data))
	}
	if finalPrompt == "" {
		return ModifyJob{}, errors.New("--prompt or --prompt-file is required")
	}

	finalMode := strings.ToLower(strings.TrimSpace(mode))
	if finalMode == "" {
		finalMode = "fast"
	}
	switch finalMode {
	case "fast", "best":
	default:
		return ModifyJob{}, fmt.Errorf("unsupported mode: %s", finalMode)
	}

	finalOutputDir := strings.TrimSpace(outDir)
	if finalOutputDir == "" {
		finalOutputDir = filepath.Dir(sourceFile)
		if finalOutputDir == "" || finalOutputDir == "." {
			finalOutputDir, _ = os.Getwd()
		}
	}

	return ModifyJob{
		SourceFilePath: sourceFile,
		DocumentType:   docType,
		Prompt:         finalPrompt,
		OutputDir:      finalOutputDir,
		Mode:           finalMode,
		JSONOutput:     jsonOutput,
	}, nil
}

func normalizeModifyFlagArgs(args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		current := args[i]
		switch current {
		case "--prompt", "--prompt-file", "--mode", "--out":
			flags = append(flags, current)
			if i+1 < len(args) {
				flags = append(flags, args[i+1])
				i += 2
				continue
			}
			i++
		case "--json":
			flags = append(flags, current)
			i++
		default:
			if strings.HasPrefix(current, "--prompt=") ||
				strings.HasPrefix(current, "--prompt-file=") ||
				strings.HasPrefix(current, "--mode=") ||
				strings.HasPrefix(current, "--out=") ||
				strings.HasPrefix(current, "--json=") {
				flags = append(flags, current)
			} else {
				positionals = append(positionals, current)
			}
			i++
		}
	}
	return append(flags, positionals...)
}
