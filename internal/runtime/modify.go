package runtime

import (
	"context"

	"github.com/officecli/officecli-internal/engine"
	"github.com/officecli/officecli-internal/internal/runtime/modify"
)

type ModifyParams = modify.Params
type ModifyResult = modify.Result
type ModifyResultMeta = modify.ResultMeta

func (s *Service) Modify(ctx context.Context, p ModifyParams, progress engine.ProgressEmitter) (*ModifyResult, error) {
	return s.modifier.Modify(ctx, p, progress)
}
