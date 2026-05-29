package ooxmledit

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	coreTimestampFieldRegex = regexp.MustCompile(`(?s)<(?:[A-Za-z0-9_]+:)?(?:created|modified)\b[^>]*(?:/>|>.*?</(?:[A-Za-z0-9_]+:)?(?:created|modified)>)`)
	appProducerFieldRegex   = regexp.MustCompile(`(?s)<(?:[A-Za-z0-9_]+:)?(?:Application|AppVersion)\b[^>]*(?:/>|>.*?</(?:[A-Za-z0-9_]+:)?(?:Application|AppVersion)>)`)
	normalizedZipTime       = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
)

// NormalizePackedForCompare rewrites an OOXML package into a stable comparison
// form by stripping known producer/timestamp fields and zeroing zip mtimes.
func NormalizePackedForCompare(ooxmlBytes []byte) []byte {
	files, err := ZipEntries(ooxmlBytes)
	if err != nil {
		return ooxmlBytes
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, f := range files {
		content, err := readZipEntryBytes(f)
		if err != nil {
			return ooxmlBytes
		}
		content = normalizePackedEntry(f.Name, content)

		header := &zip.FileHeader{
			Name:   f.Name,
			Method: zip.Deflate,
		}
		header.SetModTime(normalizedZipTime)
		out, err := writer.CreateHeader(header)
		if err != nil {
			return ooxmlBytes
		}
		if _, err := out.Write(content); err != nil {
			return ooxmlBytes
		}
	}
	if err := writer.Close(); err != nil {
		return ooxmlBytes
	}
	return buf.Bytes()
}

// NormalizeXMLForDiff canonicalizes XML enough for untouched-region diffs:
// attributes are sorted and non-preserved character whitespace is collapsed.
func NormalizeXMLForDiff(xmlBytes []byte) []byte {
	decoder := xml.NewDecoder(bytes.NewReader(xmlBytes))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	preserveStack := []bool{false}

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return bytes.TrimSpace(xmlBytes)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			sortAttrs(t.Attr)
			preserve := preserveStack[len(preserveStack)-1]
			for _, attr := range t.Attr {
				if isXMLSpaceAttr(attr.Name) {
					preserve = attr.Value == "preserve"
				}
			}
			preserveStack = append(preserveStack, preserve)
			if err := encoder.EncodeToken(t); err != nil {
				return bytes.TrimSpace(xmlBytes)
			}
		case xml.EndElement:
			if err := encoder.EncodeToken(t); err != nil {
				return bytes.TrimSpace(xmlBytes)
			}
			if len(preserveStack) > 1 {
				preserveStack = preserveStack[:len(preserveStack)-1]
			}
		case xml.CharData:
			text := string([]byte(t))
			if !preserveStack[len(preserveStack)-1] {
				text = strings.Join(strings.Fields(text), " ")
				if text == "" {
					continue
				}
			}
			if err := encoder.EncodeToken(xml.CharData([]byte(text))); err != nil {
				return bytes.TrimSpace(xmlBytes)
			}
		case xml.ProcInst, xml.Directive, xml.Comment:
			continue
		default:
			if err := encoder.EncodeToken(tok); err != nil {
				return bytes.TrimSpace(xmlBytes)
			}
		}
	}
	if err := encoder.Flush(); err != nil {
		return bytes.TrimSpace(xmlBytes)
	}
	return buf.Bytes()
}

func normalizePackedEntry(name string, content []byte) []byte {
	switch name {
	case "docProps/core.xml":
		return coreTimestampFieldRegex.ReplaceAll(content, nil)
	case "docProps/app.xml":
		return appProducerFieldRegex.ReplaceAll(content, nil)
	default:
		return content
	}
}

func readZipEntryBytes(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", f.Name, err)
	}
	defer rc.Close()
	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", f.Name, err)
	}
	return content, nil
}

func sortAttrs(attrs []xml.Attr) {
	sort.Slice(attrs, func(i, j int) bool {
		left := attrSortKey(attrs[i])
		right := attrSortKey(attrs[j])
		if left == right {
			return attrs[i].Value < attrs[j].Value
		}
		return left < right
	})
}

func attrSortKey(attr xml.Attr) string {
	return attr.Name.Space + "\x00" + attr.Name.Local
}

func isXMLSpaceAttr(name xml.Name) bool {
	return name.Local == "space" && (name.Space == "xml" || name.Space == "http://www.w3.org/XML/1998/namespace")
}
