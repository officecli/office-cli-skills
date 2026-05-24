package contentfilter

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestApply_T1_SummaryTruncatedAt500(t *testing.T) {
	// Use non-spammy content that won't trigger repeated-char rules.
	in := Input{Summary: buildNonSpamString(600)}
	out, rpt, err := Apply(in)
	require.NoError(t, err)
	require.Equal(t, 500, utf8.RuneCountInString(out.Summary))
	require.Contains(t, rpt.TruncatedFields, "summary")
}

func TestApply_T2_DetailTruncatedAt2000(t *testing.T) {
	in := Input{Detail: buildNonSpamString(2500)}
	out, rpt, err := Apply(in)
	require.NoError(t, err)
	require.Equal(t, 2000, utf8.RuneCountInString(out.Detail))
	require.Contains(t, rpt.TruncatedFields, "detail")
}

func TestApply_T3_ScriptTagTruncated(t *testing.T) {
	in := Input{Summary: "hello world<script>alert(1)</script>"}
	out, rpt, err := Apply(in)
	require.NoError(t, err)
	require.Equal(t, "hello world", out.Summary)
	require.Contains(t, rpt.TruncatedFields, "summary")
}

func TestApply_T4_JavascriptURITruncated(t *testing.T) {
	in := Input{Detail: "click here javascript:void(0) end"}
	out, rpt, err := Apply(in)
	require.NoError(t, err)
	require.Equal(t, "click here ", out.Detail)
	require.Contains(t, rpt.TruncatedFields, "detail")
}

func TestApply_T5_OnerrorTruncated(t *testing.T) {
	in := Input{Summary: "img onerror= bad"}
	out, rpt, err := Apply(in)
	require.NoError(t, err)
	require.Equal(t, "img ", out.Summary)
	require.Contains(t, rpt.TruncatedFields, "summary")
}

func TestApply_T6_RepeatedCharsTruncated(t *testing.T) {
	in := Input{Detail: "prefix " + strings.Repeat("a", 20) + " suffix"}
	out, rpt, err := Apply(in)
	require.NoError(t, err)
	require.Equal(t, "prefix ", out.Detail)
	require.Contains(t, rpt.TruncatedFields, "detail")
}

func TestApply_T7_HighRepetitionStructureTruncated(t *testing.T) {
	// "abc" repeated 11 times = high-repetition pattern
	in := Input{Detail: "start " + strings.Repeat("abc", 11)}
	out, rpt, err := Apply(in)
	require.NoError(t, err)
	require.Equal(t, "start ", out.Detail)
	require.Contains(t, rpt.TruncatedFields, "detail")
}

func TestApply_T8_ContactEmailTooLong(t *testing.T) {
	in := Input{ContactEmail: strings.Repeat("x", 300)}
	_, _, err := Apply(in)
	require.ErrorIs(t, err, ErrContactEmailTooLong)
}

func TestApply_T9_ContactEmailOK(t *testing.T) {
	in := Input{ContactEmail: "ok@x.com"}
	out, _, err := Apply(in)
	require.NoError(t, err)
	require.Equal(t, "ok@x.com", out.ContactEmail)
}

func TestApply_T10_AllFieldsEmpty(t *testing.T) {
	out, rpt, err := Apply(Input{})
	require.NoError(t, err)
	require.Equal(t, Output{}, out)
	require.Empty(t, rpt.TruncatedFields)
}

func TestApply_T11_UnicodeRuneCount(t *testing.T) {
	// 600 distinct Chinese characters should be truncated to 500 runes.
	in := Input{Summary: buildNonSpamChinese(600)}
	out, rpt, err := Apply(in)
	require.NoError(t, err)
	require.Equal(t, 500, utf8.RuneCountInString(out.Summary))
	require.Contains(t, rpt.TruncatedFields, "summary")
}

func TestApply_T12_LengthAndRegexDoubleHit(t *testing.T) {
	// Build a 600-char summary that, after length truncation to 500, still contains
	// a script tag at position 10.
	prefix := "safe text "                                                          // 10 chars
	inject := "<script>x"                                                           // 9 chars
	filler := buildNonSpamString(600 - utf8.RuneCountInString(prefix+inject))       // fills to 600
	in := Input{Summary: prefix + inject + filler}

	out, rpt, err := Apply(in)
	require.NoError(t, err)
	// Length truncation fires first (600 -> 500), then regex truncates at "<script>".
	require.Equal(t, "safe text ", out.Summary)
	require.Contains(t, rpt.TruncatedFields, "summary")
}

// buildNonSpamString returns a string of exactly n runes that won't trigger
// spam-detection rules. Uses incrementing numbers to avoid any repeating pattern.
func buildNonSpamString(n int) string {
	var b strings.Builder
	for i := 0; b.Len() < n*4; i++ { // over-generate, then truncate by rune
		fmt.Fprintf(&b, "%d ", i)
	}
	return truncateTestRunes(b.String(), n)
}

// buildNonSpamChinese returns a string of exactly n Chinese runes that won't
// trigger spam detection. Uses CJK Unified Ideographs starting at U+4E00.
func buildNonSpamChinese(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(rune(0x4E00 + (i % 1000))) // 1000 distinct chars
	}
	return b.String()
}

func truncateTestRunes(s string, n int) string {
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
