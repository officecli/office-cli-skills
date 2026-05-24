package contentfilter

import (
	"errors"
	"regexp"
	"unicode/utf8"
)

// Field length limits (rune count).
const (
	maxSummaryRunes      = 500
	maxDetailRunes       = 2000
	maxCategoryRunes     = 64
	maxErrorMessageRunes = 500
	maxContactEmailBytes = 254
)

// ErrContactEmailTooLong is returned when contact_email exceeds 254 characters.
var ErrContactEmailTooLong = errors.New("contact_email exceeds 254 chars")

// Compiled deny-list patterns (case-insensitive) for script injection.
var denyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<script[^>]*>`),
	regexp.MustCompile(`(?i)javascript:`),
	regexp.MustCompile(`(?i)onerror\s*=`),
}

// Input holds the raw text fields from a desktop issue-report payload.
type Input struct {
	Summary      string
	Detail       string
	Category     string
	ErrorMessage string
	ContactEmail string
}

// Output holds the filtered (possibly truncated) text fields.
type Output struct {
	Summary      string
	Detail       string
	Category     string
	ErrorMessage string
	ContactEmail string // unchanged (length-validated, not truncated)
}

// Report describes which fields were modified by the filter.
type Report struct {
	// TruncatedFields lists field names that were truncated by length or regex.
	TruncatedFields []string // e.g. ["summary", "detail"]
}

// Apply applies content-filter rules to the given Input.
// Order: length truncation first, then regex deny-pattern truncation.
// contact_email is strict: if it exceeds 254 chars, ErrContactEmailTooLong is returned.
func Apply(in Input) (Output, Report, error) {
	// Validate contact_email strictly (byte length, per RFC 5321).
	if in.ContactEmail != "" && utf8.RuneCountInString(in.ContactEmail) > maxContactEmailBytes {
		return Output{}, Report{}, ErrContactEmailTooLong
	}

	var truncated []string

	summary, t := filterField(in.Summary, maxSummaryRunes)
	if t {
		truncated = appendUnique(truncated, "summary")
	}

	detail, t := filterField(in.Detail, maxDetailRunes)
	if t {
		truncated = appendUnique(truncated, "detail")
	}

	category, t := filterField(in.Category, maxCategoryRunes)
	if t {
		truncated = appendUnique(truncated, "category")
	}

	errorMessage, t := filterField(in.ErrorMessage, maxErrorMessageRunes)
	if t {
		truncated = appendUnique(truncated, "error_message")
	}

	return Output{
		Summary:      summary,
		Detail:       detail,
		Category:     category,
		ErrorMessage: errorMessage,
		ContactEmail: in.ContactEmail,
	}, Report{TruncatedFields: truncated}, nil
}

// filterField applies length truncation then regex deny-pattern truncation.
// Returns the (possibly truncated) string and whether any truncation occurred.
func filterField(s string, maxRunes int) (string, bool) {
	if s == "" {
		return s, false
	}

	truncated := false

	// Step 1: length truncation (by rune count).
	if utf8.RuneCountInString(s) > maxRunes {
		s = truncateRunes(s, maxRunes)
		truncated = true
	}

	// Step 2: regex deny-pattern truncation.
	for _, pat := range denyPatterns {
		loc := pat.FindStringIndex(s)
		if loc != nil {
			s = s[:loc[0]]
			truncated = true
			break
		}
	}

	// Step 3: spam pattern truncation (backreference-equivalent checks).
	if idx := findRepeatedCharRun(s, 20); idx >= 0 {
		s = s[:idx]
		truncated = true
	}
	if idx := findHighRepetition(s, 30, 11); idx >= 0 {
		s = s[:idx]
		truncated = true
	}

	return s, truncated
}

// findRepeatedCharRun returns the byte index where a run of n or more identical
// runes starts, or -1 if no such run exists.
func findRepeatedCharRun(s string, n int) int {
	if len(s) == 0 {
		return -1
	}
	runStart := 0
	count := 1
	prevRune := rune(-1)
	for i, r := range s {
		if r == prevRune {
			count++
			if count >= n {
				return runStart
			}
		} else {
			runStart = i
			count = 1
		}
		prevRune = r
	}
	return -1
}

// findHighRepetition returns the byte index where a repeating block of length
// 1..maxBlock that repeats minRepeats or more consecutive times starts, or -1.
func findHighRepetition(s string, maxBlock, minRepeats int) int {
	runes := []rune(s)
	n := len(runes)
	for blockLen := 1; blockLen <= maxBlock && blockLen <= n; blockLen++ {
		for start := 0; start+blockLen*minRepeats <= n; start++ {
			block := string(runes[start : start+blockLen])
			repeats := 1
			pos := start + blockLen
			for pos+blockLen <= n && string(runes[pos:pos+blockLen]) == block {
				repeats++
				pos += blockLen
			}
			if repeats >= minRepeats {
				// Convert rune index back to byte index.
				return runeIndexToByteIndex(s, start)
			}
		}
	}
	return -1
}

// truncateRunes returns the first n runes of s.
func truncateRunes(s string, n int) string {
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// runeIndexToByteIndex converts a rune index to the corresponding byte index in s.
func runeIndexToByteIndex(s string, runeIdx int) int {
	count := 0
	for i := range s {
		if count == runeIdx {
			return i
		}
		count++
	}
	return len(s)
}

// appendUnique appends v to slice only if it is not already present.
func appendUnique(slice []string, v string) []string {
	for _, existing := range slice {
		if existing == v {
			return slice
		}
	}
	return append(slice, v)
}
