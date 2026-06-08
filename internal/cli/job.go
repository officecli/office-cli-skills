package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/officecli/officecli/engine"
	"github.com/officecli/officecli/internal/runtime"
)

type boolValue struct {
	set   bool
	value bool
}

func (b *boolValue) String() string {
	if b == nil {
		return ""
	}
	if b.value {
		return "true"
	}
	return "false"
}

func (b *boolValue) Set(value string) error {
	b.set = true
	if strings.TrimSpace(value) == "" {
		b.value = true
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		b.value = true
	case "0", "false", "no", "off":
		b.value = false
	default:
		return fmt.Errorf("invalid boolean value: %s", value)
	}
	return nil
}

func (b *boolValue) IsBoolFlag() bool { return true }

type stringList struct {
	values []string
}

const maxExplicitReferencePPTXBytes = 100 << 20

func (s *stringList) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(s.values, ",")
}

func (s *stringList) Set(value string) error {
	s.values = append(s.values, value)
	return nil
}

func (s *stringList) Slice() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.values))
	for _, v := range s.values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func BuildGenerateJob(args []string, cfg Config, src InputSources) (GenerateJob, error) {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})

	var prompt string
	var promptFile string
	var mode string
	var runtimeMode string
	var lang string
	var style string
	var audience string
	var outDir string
	var sourceFile string
	var ratio string
	var referenceImages stringList
	var referenceRoot string
	var referencePPTXs stringList
	var pptxBackend string
	var imageSize string
	var imageQuality string
	var gifFPS int
	var jsonOutput bool
	var localPreview bool
	var debug bool
	var publishFlag boolValue
	var noPublishFlag boolValue
	var noImagesFlag boolValue
	var noReferenceScanFlag boolValue

	fs.StringVar(&prompt, "prompt", "", "")
	fs.StringVar(&promptFile, "prompt-file", "", "")
	fs.StringVar(&mode, "mode", "", "")
	fs.StringVar(&runtimeMode, "runtime-mode", "", "")
	fs.StringVar(&lang, "lang", "", "")
	fs.StringVar(&style, "style", "", "")
	fs.StringVar(&audience, "audience", "", "")
	fs.StringVar(&outDir, "out", "", "")
	fs.StringVar(&sourceFile, "file", "", "")
	fs.StringVar(&ratio, "ratio", "", "")
	fs.Var(&referenceImages, "reference-image", "")
	fs.StringVar(&referenceRoot, "reference-root", "", "")
	fs.Var(&referencePPTXs, "reference-pptx", "")
	fs.StringVar(&pptxBackend, "pptx-backend", "", "")
	fs.StringVar(&imageSize, "size", "", "")
	fs.StringVar(&imageQuality, "image-quality", "", "")
	fs.IntVar(&gifFPS, "fps", 0, "")
	fs.BoolVar(&jsonOutput, "json", false, "")
	fs.BoolVar(&localPreview, "local-preview", false, "")
	fs.BoolVar(&debug, "debug", false, "")
	fs.Var(&publishFlag, "publish", "")
	fs.Var(&noPublishFlag, "no-publish", "")
	fs.Var(&noImagesFlag, "no-images", "")
	fs.Var(&noReferenceScanFlag, "no-reference-scan", "")

	normalizedArgs := normalizeFlagArgs(args)
	if err := fs.Parse(normalizedArgs); err != nil {
		return GenerateJob{}, err
	}
	positionals := fs.Args()
	if len(positionals) == 0 {
		return GenerateJob{}, errors.New("document type is required")
	}

	documentType, err := parseDocumentType(positionals[0])
	if err != nil {
		return GenerateJob{}, err
	}
	if len(positionals) < 2 {
		return GenerateJob{}, errors.New("topic is required")
	}
	topic := strings.TrimSpace(positionals[1])
	brief := ""
	if len(positionals) > 2 {
		brief = strings.TrimSpace(strings.Join(positionals[2:], " "))
	}
	if topic == "" {
		return GenerateJob{}, errors.New("topic is required")
	}
	sourceFile = strings.TrimSpace(sourceFile)

	if strings.TrimSpace(prompt) != "" && strings.TrimSpace(promptFile) != "" {
		return GenerateJob{}, errors.New("use only one of --prompt and --prompt-file")
	}
	finalPrompt := strings.TrimSpace(prompt)
	if finalPrompt == "" && strings.TrimSpace(promptFile) != "" {
		data, err := os.ReadFile(promptFile)
		if err != nil {
			return GenerateJob{}, fmt.Errorf("read prompt file: %w", err)
		}
		finalPrompt = strings.TrimSpace(string(data))
	}
	if finalPrompt == "" && strings.TrimSpace(src.Stdin) != "" {
		finalPrompt = strings.TrimSpace(src.Stdin)
	}
	if finalPrompt == "" {
		if brief != "" {
			finalPrompt = brief
		} else {
			finalPrompt = topic
		}
	}

	modeSpecified := strings.TrimSpace(mode) != ""
	finalMode := strings.TrimSpace(mode)
	if isStandaloneImageDocumentType(documentType) && finalMode == "" {
		finalMode = "fast"
	} else if finalMode == "" {
		finalMode = strings.TrimSpace(cfg.Defaults.Mode)
	}
	if finalMode == "" {
		finalMode = "best"
	}
	switch strings.ToLower(finalMode) {
	case "fast", "best":
	default:
		return GenerateJob{}, fmt.Errorf("unsupported mode: %s", finalMode)
	}
	if isStandaloneImageDocumentType(documentType) && modeSpecified && strings.EqualFold(finalMode, "best") {
		return GenerateJob{}, fmt.Errorf("--mode best is not supported for %s generation", documentType)
	}

	selectedRuntimeMode := RuntimeMode(strings.ToLower(strings.TrimSpace(runtimeMode)))
	if isStandaloneImageDocumentType(documentType) && selectedRuntimeMode == "" {
		selectedRuntimeMode = cfg.RuntimeModeOrDefault()
	} else if selectedRuntimeMode == "" {
		selectedRuntimeMode = cfg.RuntimeModeOrDefault()
	}
	if selectedRuntimeMode == "" {
		selectedRuntimeMode = RuntimeModeHosted
	}
	switch selectedRuntimeMode {
	case RuntimeModeExternal, RuntimeModeHosted:
	default:
		return GenerateJob{}, fmt.Errorf("unsupported runtime mode: %s", selectedRuntimeMode)
	}

	finalOutputDir := strings.TrimSpace(outDir)
	if finalOutputDir == "" {
		finalOutputDir = strings.TrimSpace(cfg.Defaults.OutputDir)
	}
	if finalOutputDir == "" {
		base := src.CWD
		if base == "" {
			base, _ = os.Getwd()
		}
		finalOutputDir = filepath.Join(base, "output")
	}

	publishEnabled := cfg.Defaults.Publish
	if isStandaloneImageDocumentType(documentType) {
		publishEnabled = true
	}
	if publishFlag.set {
		publishEnabled = publishFlag.value
	}
	if noPublishFlag.set && noPublishFlag.value {
		publishEnabled = false
	}
	enableImages := true
	if noImagesFlag.set && noImagesFlag.value {
		enableImages = false
	}
	ratioSpecified := strings.TrimSpace(ratio) != ""
	if ratioSpecified && documentType != engine.DocumentTypeIMG {
		return GenerateJob{}, fmt.Errorf("--ratio is only supported for img generation")
	}
	referenceImageList := referenceImages.Slice()
	if len(referenceImageList) > 0 && !isStandaloneImageDocumentType(documentType) {
		return GenerateJob{}, fmt.Errorf("--reference-image is only supported for img or gif generation")
	}
	referenceRoot = strings.TrimSpace(referenceRoot)
	referencePPTXList := referencePPTXs.Slice()
	if referenceRoot != "" && documentType != engine.DocumentTypePPTX {
		return GenerateJob{}, fmt.Errorf("--reference-root is only supported for pptx generation")
	}
	if len(referencePPTXList) > 0 && documentType != engine.DocumentTypePPTX {
		return GenerateJob{}, fmt.Errorf("--reference-pptx is only supported for pptx generation")
	}
	if noReferenceScanFlag.set && documentType != engine.DocumentTypePPTX {
		return GenerateJob{}, fmt.Errorf("--no-reference-scan is only supported for pptx generation")
	}
	if documentType == engine.DocumentTypePPTX {
		if err := validateExplicitReferencePPTXFiles(referencePPTXList); err != nil {
			return GenerateJob{}, err
		}
	}
	pptxBackend = strings.TrimSpace(pptxBackend)
	if pptxBackend != "" && documentType != engine.DocumentTypePPTX {
		return GenerateJob{}, fmt.Errorf("--pptx-backend is only supported for pptx generation")
	}
	finalPPTXBackend := ""
	if documentType == engine.DocumentTypePPTX {
		var err error
		finalPPTXBackend, err = runtime.NormalizePPTXBackend(pptxBackend)
		if err != nil {
			return GenerateJob{}, err
		}
	}
	finalImageSize := strings.TrimSpace(imageSize)
	if finalImageSize != "" && documentType != engine.DocumentTypeIMG {
		return GenerateJob{}, fmt.Errorf("--size is only supported for img generation")
	}
	if finalImageSize != "" {
		if _, _, err := parseImageSize(finalImageSize); err != nil {
			return GenerateJob{}, err
		}
	}
	// --image-quality 标志已废弃：仍解析以保持向后兼容，但取值被忽略，永远走 hosted image 路径。
	_ = imageQuality
	imageRatio := strings.ToLower(strings.TrimSpace(ratio))
	if imageRatio == "" {
		imageRatio = "square"
	}
	switch imageRatio {
	case "square", "landscape", "portrait":
	default:
		return GenerateJob{}, fmt.Errorf("unsupported image ratio: %s", ratio)
	}
	if gifFPS != 0 && documentType != engine.DocumentTypeGIF {
		return GenerateJob{}, fmt.Errorf("--fps is only supported for gif generation")
	}
	if documentType == engine.DocumentTypeGIF {
		if gifFPS == 0 {
			gifFPS = 16
		}
		if gifFPS < 4 || gifFPS > 24 {
			return GenerateJob{}, fmt.Errorf("--fps must be between 4 and 24")
		}
		imageRatio = "square"
		finalImageSize = "1024x1024"
	}
	finalStyle := strings.TrimSpace(style)
	styleSpecified := finalStyle != ""
	if finalStyle == "" && documentType == engine.DocumentTypePPTX {
		finalStyle = strings.TrimSpace(cfg.Defaults.PPTXStylePreset)
	}
	referenceScanEnabled := false
	finalReferenceRoot := referenceRoot
	if documentType == engine.DocumentTypePPTX {
		referenceScanEnabled = true
		if noReferenceScanFlag.set && noReferenceScanFlag.value {
			referenceScanEnabled = false
		}
		if finalReferenceRoot == "" {
			finalReferenceRoot = strings.TrimSpace(src.CWD)
		}
		if finalReferenceRoot == "" {
			finalReferenceRoot, _ = os.Getwd()
		}
	}
	var warnings []engine.GenerateIssue
	if isStandaloneImageDocumentType(documentType) {
		switch {
		case sourceFile != "":
			return GenerateJob{}, fmt.Errorf("--file is not supported for %s generation", documentType)
		case localPreview:
			return GenerateJob{}, fmt.Errorf("--local-preview is not supported for %s generation", documentType)
		case noImagesFlag.set && noImagesFlag.value:
			return GenerateJob{}, fmt.Errorf("--no-images is not supported for %s generation", documentType)
		}
	} else if documentType == engine.DocumentTypeReport {
		if sourceFile == "" {
			return GenerateJob{}, errors.New("report generation requires --file <xlsx-path>")
		}
		if strings.ToLower(filepath.Ext(sourceFile)) != ".xlsx" {
			return GenerateJob{}, fmt.Errorf("report --file must point to an .xlsx workbook: %s", sourceFile)
		}
	} else if sourceFile != "" {
		return GenerateJob{}, fmt.Errorf("--file is only supported for report generation")
	}

	return GenerateJob{
		DocumentType:          documentType,
		Topic:                 topic,
		Brief:                 brief,
		OriginalPrompt:        finalPrompt,
		Prompt:                finalPrompt,
		SourceFilePath:        sourceFile,
		RuntimeMode:           selectedRuntimeMode,
		Mode:                  strings.ToLower(finalMode),
		Language:              strings.TrimSpace(lang),
		Style:                 finalStyle,
		StyleSpecified:        styleSpecified,
		Audience:              strings.TrimSpace(audience),
		EnableImages:          enableImages,
		ImageRatio:            imageRatio,
		ImageSize:             finalImageSize,
		GifFPS:                gifFPS,
		ReferenceImageSources: referenceImageList,
		ReferenceScanEnabled:  referenceScanEnabled,
		ReferenceScanRoot:     finalReferenceRoot,
		ReferencePPTXSources:  referencePPTXList,
		PPTXBackend:           finalPPTXBackend,
		LocalPreview:          localPreview,
		OutputDir:             finalOutputDir,
		Publish:               publishEnabled,
		Debug:                 debug,
		JSONOutput:            jsonOutput,
		Warnings:              warnings,
	}, nil
}

func validateExplicitReferencePPTXFiles(sources []string) error {
	for _, source := range sources {
		path := strings.TrimSpace(source)
		if path == "" {
			continue
		}
		if !strings.EqualFold(filepath.Ext(path), ".pptx") {
			return fmt.Errorf("reference pptx must point to a .pptx file: %s", path)
		}
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("reference pptx does not exist: %s", path)
			}
			return fmt.Errorf("read reference pptx metadata: %w", err)
		}
		if info.IsDir() {
			return fmt.Errorf("reference pptx must point to a file: %s", path)
		}
		if info.Size() > maxExplicitReferencePPTXBytes {
			return fmt.Errorf("reference pptx exceeds %d bytes: %s", maxExplicitReferencePPTXBytes, path)
		}
	}
	return nil
}

func normalizeFlagArgs(args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		current := args[i]
		switch current {
		case "--prompt", "--prompt-file", "--mode", "--runtime-mode", "--lang", "--style", "--audience", "--out", "--file", "--ratio", "--reference-image", "--reference-root", "--reference-pptx", "--pptx-backend", "--size", "--image-quality", "--fps", "--fail-below":
			flags = append(flags, current)
			if i+1 < len(args) {
				flags = append(flags, args[i+1])
				i += 2
				continue
			}
			i++
		case "--json", "--publish", "--no-publish", "--no-images", "--no-reference-scan", "--no-visual", "--local-preview", "--debug":
			flags = append(flags, current)
			i++
		default:
			if strings.HasPrefix(current, "--prompt=") ||
				strings.HasPrefix(current, "--prompt-file=") ||
				strings.HasPrefix(current, "--mode=") ||
				strings.HasPrefix(current, "--runtime-mode=") ||
				strings.HasPrefix(current, "--lang=") ||
				strings.HasPrefix(current, "--style=") ||
				strings.HasPrefix(current, "--audience=") ||
				strings.HasPrefix(current, "--out=") ||
				strings.HasPrefix(current, "--file=") ||
				strings.HasPrefix(current, "--ratio=") ||
				strings.HasPrefix(current, "--reference-image=") ||
				strings.HasPrefix(current, "--reference-root=") ||
				strings.HasPrefix(current, "--reference-pptx=") ||
				strings.HasPrefix(current, "--pptx-backend=") ||
				strings.HasPrefix(current, "--size=") ||
				strings.HasPrefix(current, "--image-quality=") ||
				strings.HasPrefix(current, "--fps=") ||
				strings.HasPrefix(current, "--fail-below=") ||
				strings.HasPrefix(current, "--publish=") ||
				strings.HasPrefix(current, "--no-publish=") ||
				strings.HasPrefix(current, "--no-images=") ||
				strings.HasPrefix(current, "--no-reference-scan=") ||
				strings.HasPrefix(current, "--no-visual=") ||
				strings.HasPrefix(current, "--local-preview=") ||
				strings.HasPrefix(current, "--debug=") {
				flags = append(flags, current)
			} else {
				positionals = append(positionals, current)
			}
			i++
		}
	}
	return append(flags, positionals...)
}

func countReferenceImageFlags(args []string) int {
	count := 0
	for _, arg := range args {
		if arg == "--reference-image" || strings.HasPrefix(arg, "--reference-image=") {
			count++
		}
	}
	return count
}

func parseImageSize(value string) (int, int, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, 0, nil
	}
	parts := strings.SplitN(value, "x", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("--size must be in WxH form (e.g. 1280x768): %s", value)
	}
	w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || w <= 0 {
		return 0, 0, fmt.Errorf("--size width is invalid: %s", value)
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || h <= 0 {
		return 0, 0, fmt.Errorf("--size height is invalid: %s", value)
	}
	return w, h, nil
}

func parseDocumentType(value string) (engine.DocumentType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pptx":
		return engine.DocumentTypePPTX, nil
	case "docx":
		return engine.DocumentTypeDOCX, nil
	case "xlsx":
		return engine.DocumentTypeXLSX, nil
	case "report":
		return engine.DocumentTypeReport, nil
	case "img", "image":
		return engine.DocumentTypeIMG, nil
	case "gif":
		return engine.DocumentTypeGIF, nil
	default:
		return "", fmt.Errorf("unsupported document type: %s", value)
	}
}

func isStandaloneImageDocumentType(documentType engine.DocumentType) bool {
	return documentType == engine.DocumentTypeIMG || documentType == engine.DocumentTypeGIF
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
