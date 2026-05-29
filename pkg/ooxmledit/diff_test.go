package ooxmledit

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestNormalizeXMLForDiffSortsAttributes(t *testing.T) {
	left := NormalizeXMLForDiff([]byte(`<root><item z="last" a="first" m="middle">value</item></root>`))
	right := NormalizeXMLForDiff([]byte(`<root><item a="first" m="middle" z="last">value</item></root>`))

	if !bytes.Equal(left, right) {
		t.Fatalf("normalized XML differs:\nleft:  %s\nright: %s", left, right)
	}
}

func TestNormalizeXMLForDiffPreservesXMLSpace(t *testing.T) {
	got := NormalizeXMLForDiff([]byte(`<root><t xml:space="preserve">  keep   spacing  </t><t>  collapse   spacing  </t></root>`))

	if !bytes.Contains(got, []byte(`>  keep   spacing  <`)) {
		t.Fatalf("xml:space=preserve text was collapsed: %s", got)
	}
	if strings.Contains(string(got), `>  collapse   spacing  <`) {
		t.Fatalf("non-preserve whitespace was not collapsed: %s", got)
	}
	if !bytes.Contains(got, []byte(`>collapse spacing<`)) {
		t.Fatalf("collapsed text not found: %s", got)
	}
}

func TestNormalizePackedForCompareStripsEnumeratedDocFields(t *testing.T) {
	left := buildZipWithTimes(t, time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), map[string][]byte{
		"docProps/core.xml": []byte(`<cp:coreProperties xmlns:cp="cp" xmlns:dcterms="dcterms"><dcterms:created>2024-01-02T03:04:05Z</dcterms:created><dcterms:modified>2024-01-03T03:04:05Z</dcterms:modified><dc:title xmlns:dc="dc">Quarterly Plan</dc:title></cp:coreProperties>`),
		"docProps/app.xml":  []byte(`<Properties><Application>Excel</Application><AppVersion>16.0000</AppVersion><Company>OfficeCLI</Company></Properties>`),
		"word/document.xml": []byte(`<w:document><w:body><w:p>same</w:p></w:body></w:document>`),
	})
	right := buildZipWithTimes(t, time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC), map[string][]byte{
		"docProps/core.xml": []byte(`<cp:coreProperties xmlns:cp="cp" xmlns:dcterms="dcterms"><dcterms:created>2026-05-06T07:08:09Z</dcterms:created><dcterms:modified>2026-05-07T07:08:09Z</dcterms:modified><dc:title xmlns:dc="dc">Quarterly Plan</dc:title></cp:coreProperties>`),
		"docProps/app.xml":  []byte(`<Properties><Application>OfficeCLI</Application><AppVersion>99.9999</AppVersion><Company>OfficeCLI</Company></Properties>`),
		"word/document.xml": []byte(`<w:document><w:body><w:p>same</w:p></w:body></w:document>`),
	})

	normalizedLeft := NormalizePackedForCompare(left)
	normalizedRight := NormalizePackedForCompare(right)
	if !bytes.Equal(normalizedLeft, normalizedRight) {
		t.Fatalf("normalized packages differ after stripping enumerated fields")
	}

	core := readZipEntryBytesFromArchive(t, normalizedLeft, "docProps/core.xml")
	if bytes.Contains(core, []byte("created")) || bytes.Contains(core, []byte("modified")) {
		t.Fatalf("core timestamp fields were not stripped: %s", core)
	}
	app := readZipEntryBytesFromArchive(t, normalizedLeft, "docProps/app.xml")
	if bytes.Contains(app, []byte("Application")) || bytes.Contains(app, []byte("AppVersion")) {
		t.Fatalf("app producer fields were not stripped: %s", app)
	}
}

func buildZipWithTimes(t *testing.T, modTime time.Time, entries map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range entries {
		header := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		header.SetModTime(modTime)
		f, err := w.CreateHeader(header)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := f.Write(content); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func readZipEntryBytesFromArchive(t *testing.T, archive []byte, name string) []byte {
	t.Helper()

	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return data
	}
	t.Fatalf("entry %s not found", name)
	return nil
}
