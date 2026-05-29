package modify

import (
	"context"
	"fmt"
	"os"

	"github.com/officecli/officecli/engine"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

type Modifier struct {
	llm engine.LLMClient
}

func New(llm engine.LLMClient) *Modifier {
	return &Modifier{llm: llm}
}

func (m *Modifier) Modify(ctx context.Context, p Params, progress engine.ProgressEmitter) (*Result, error) {
	applyDefaults(&p)

	srcBytes, err := os.ReadFile(p.SourceFilePath)
	if err != nil {
		return nil, fmt.Errorf("read source file: %w", err)
	}

	fileType, err := ooxmledit.GetFileType(p.SourceFilePath)
	if err != nil {
		return nil, fmt.Errorf("detect file type: %w", err)
	}

	switch fileType {
	case ooxmledit.FileTypePPTX:
		return m.modifyPPTX(ctx, p, srcBytes, progress)
	case ooxmledit.FileTypeDOCX:
		return m.modifyDOCX(ctx, p, srcBytes, progress)
	case ooxmledit.FileTypeXLSX:
		return m.modifyXLSX(ctx, p, srcBytes, progress)
	default:
		return nil, fmt.Errorf("unsupported file type: %s", fileType)
	}
}

