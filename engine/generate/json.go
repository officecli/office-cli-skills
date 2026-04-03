package generate

import "strings"

func RepairUnescapedQuotes(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 64)
	inString := false
	for i := 0; i < len(s); {
		c := s[i]
		if !inString {
			b.WriteByte(c)
			if c == '"' {
				inString = true
			}
			i++
			continue
		}
		if c == '\\' {
			b.WriteByte(c)
			i++
			if i < len(s) {
				b.WriteByte(s[i])
				i++
			}
			continue
		}
		if c == '"' {
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j >= len(s) || s[j] == ',' || s[j] == ']' || s[j] == '}' || s[j] == ':' {
				b.WriteByte(c)
				inString = false
			} else {
				b.WriteByte('\\')
				b.WriteByte(c)
			}
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

func ExtractJSON(content string) string {
	content = strings.TrimSpace(content)
	if idx := strings.Index(content, "```json"); idx >= 0 {
		content = content[idx+7:]
		if end := strings.Index(content, "```"); end >= 0 {
			content = content[:end]
		}
	} else if idx := strings.Index(content, "```"); idx >= 0 {
		content = content[idx+3:]
		if end := strings.Index(content, "```"); end >= 0 {
			content = content[:end]
		}
	}
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}
	return content
}

func ExtractTitleFromDescription(description string) string {
	desc := strings.TrimSpace(description)
	if len([]rune(desc)) > 20 {
		runes := []rune(desc)
		return string(runes[:20])
	}
	if desc == "" {
		return "document"
	}
	return desc
}

func SanitizeFileName(name string) string {
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", "*", "-",
		"?", "-", "\"", "-", "<", "-", ">", "-", "|", "-",
		" ", "_",
	)
	return replacer.Replace(name)
}
