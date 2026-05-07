package previewshare

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderPasswordPageSizesFormControlsWithinCard(t *testing.T) {
	html := RenderPasswordPage(nil, "")

	require.Contains(t, html, `input { width: 100%; padding: 12px 14px; border-radius: 12px; border: 1px solid #cbd5e1; font-size: 16px; box-sizing: border-box; display: block; }`)
	require.Contains(t, html, `button { width: 100%; margin-top: 16px; padding: 12px 14px; border: 0; border-radius: 12px; background: #0f172a; color: #fff; font-size: 16px; cursor: pointer; box-sizing: border-box; display: block; }`)
}
