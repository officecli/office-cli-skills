package ooxmledit

import (
	"archive/zip"
	"bytes"
	"testing"
)

// createMinimalPPTX creates a minimal valid PPTX with 2 slides for testing
func createMinimalPPTX(t *testing.T) []byte {
	t.Helper()

	// Minimal slide1.xml with title placeholder
	slide1 := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld>
    <p:spTree>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="1" name="Title"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="title"/></p:nvPr>
        </p:nvSpPr>
        <p:txBody>
          <a:p><a:r><a:t>Original Title</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="2" name="Body"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="body"/></p:nvPr>
        </p:nvSpPr>
        <p:txBody>
          <a:p><a:r><a:t>Bullet 1</a:t></a:r></a:p>
          <a:p><a:r><a:t>Bullet 2</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
    </p:spTree>
  </p:cSld>
</p:sld>`

	// Minimal slide2.xml with table
	slide2 := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld>
    <p:spTree>
      <p:graphicFrame>
        <a:graphic>
          <a:graphicData>
            <a:tbl>
              <a:tr>
                <a:tc><a:txBody><a:p><a:r><a:t>Cell A1</a:t></a:r></a:p></a:txBody></a:tc>
                <a:tc><a:txBody><a:p><a:r><a:t>Cell B1</a:t></a:r></a:p></a:txBody></a:tc>
              </a:tr>
              <a:tr>
                <a:tc><a:txBody><a:p><a:r><a:t>Cell A2</a:t></a:r></a:p></a:txBody></a:tc>
                <a:tc><a:txBody><a:p><a:r><a:t>Cell B2</a:t></a:r></a:p></a:txBody></a:tc>
              </a:tr>
            </a:tbl>
          </a:graphicData>
        </a:graphic>
      </p:graphicFrame>
    </p:spTree>
  </p:cSld>
</p:sld>`

	contentXMLs := map[string]string{
		"ppt/slides/slide1.xml": slide1,
		"ppt/slides/slide2.xml": slide2,
	}

	// Create minimal PPTX structure
	pptxBytes := createTestPPTX(contentXMLs)
	return pptxBytes
}

func TestReplaceSlideTitle(t *testing.T) {
	pptx := createMinimalPPTX(t)

	// Test: Replace slide 1 title
	modified, err := ReplaceSlideTitle(pptx, 1, "New Title")
	if err != nil {
		t.Fatalf("ReplaceSlideTitle failed: %v", err)
	}

	// Verify: Extract and check
	contentXMLs, err := ExtractContentXML(modified, FileTypePPTX)
	if err != nil {
		t.Fatalf("ExtractContentXML failed: %v", err)
	}

	slide1 := contentXMLs["ppt/slides/slide1.xml"]
	if !contains(slide1, "New Title") {
		t.Errorf("Expected 'New Title' in slide1, got: %s", slide1)
	}
	if contains(slide1, "Original Title") {
		t.Errorf("Old title 'Original Title' should be replaced")
	}

	// Test: Invalid slide index
	_, err = ReplaceSlideTitle(pptx, 99, "Test")
	if err == nil {
		t.Error("Expected error for non-existent slide")
	}

	// Test: slideIndex < 1
	_, err = ReplaceSlideTitle(pptx, 0, "Test")
	if err == nil {
		t.Error("Expected error for slideIndex < 1")
	}
}

func TestExtractAndReplaceSlideTextRuns(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld>
    <p:spTree>
      <p:sp><p:txBody><a:p><a:r><a:t>植物世界</a:t></a:r></a:p></p:txBody></p:sp>
      <p:sp><p:txBody><a:p><a:r><a:t>探索森林与花卉</a:t></a:r></a:p></p:txBody></p:sp>
      <p:sp><p:txBody><a:p><a:r><a:t>Page 01</a:t></a:r></a:p></p:txBody></p:sp>
    </p:spTree>
  </p:cSld>
</p:sld>`

	runs := ExtractSlideTextRuns(xml)
	if len(runs) != 3 {
		t.Fatalf("run count = %d, want 3", len(runs))
	}
	if runs[0] != "植物世界" || runs[1] != "探索森林与花卉" || runs[2] != "Page 01" {
		t.Fatalf("unexpected runs: %#v", runs)
	}

	replaced, err := ReplaceSlideTextRuns(xml, []string{"Plant World", "Explore forests and flowers", "Page 01"})
	if err != nil {
		t.Fatalf("ReplaceSlideTextRuns failed: %v", err)
	}
	if !contains(replaced, "Plant World") || !contains(replaced, "Explore forests and flowers") {
		t.Fatalf("expected translated text in xml: %s", replaced)
	}
	if contains(replaced, "植物世界") || contains(replaced, "探索森林与花卉") {
		t.Fatalf("expected source text to be replaced: %s", replaced)
	}
}

func TestStripNonContentAttributes(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld>
    <p:spTree>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="2" name="Title"/>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm><a:off x="1500000" y="2200000"/><a:ext cx="9200000" cy="1200000"/></a:xfrm>
          <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
          <a:solidFill><a:srgbClr val="3A5A7C"/></a:solidFill>
          <a:ln w="28575"><a:solidFill><a:srgbClr val="6B9AC4"/></a:solidFill></a:ln>
        </p:spPr>
        <p:txBody>
          <a:bodyPr anchor="b"/>
          <a:lstStyle/>
          <a:p>
            <a:r>
              <a:rPr lang="zh-CN" sz="4400" b="1"><a:solidFill><a:srgbClr val="3A5A7C"/></a:solidFill></a:rPr>
              <a:t>动物世界</a:t>
            </a:r>
          </a:p>
        </p:txBody>
      </p:sp>
    </p:spTree>
  </p:cSld>
</p:sld>`

	sanitized := StripNonContentAttributes(xml)

	if !contains(sanitized, "动物世界") {
		t.Fatalf("expected visible text to remain: %s", sanitized)
	}
	if !contains(sanitized, `name="Title"`) {
		t.Fatalf("expected shape semantic name to remain: %s", sanitized)
	}
	if contains(sanitized, "prstGeom") || contains(sanitized, "srgbClr") || contains(sanitized, "solidFill") || contains(sanitized, "a:xfrm") {
		t.Fatalf("expected non-content formatting nodes to be removed: %s", sanitized)
	}
}

func TestReplaceSlideTextRunsCountMismatch(t *testing.T) {
	xml := `<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>A</a:t></a:r></a:p><a:p><a:r><a:t>B</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
	if _, err := ReplaceSlideTextRuns(xml, []string{"Only One"}); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestAppendSlideBullets(t *testing.T) {
	pptx := createMinimalPPTX(t)

	// Test: Append bullets to slide 1
	modified, err := AppendSlideBullets(pptx, 1, []string{"Bullet 3", "Bullet 4"})
	if err != nil {
		t.Fatalf("AppendSlideBullets failed: %v", err)
	}

	contentXMLs, err := ExtractContentXML(modified, FileTypePPTX)
	if err != nil {
		t.Fatalf("ExtractContentXML failed: %v", err)
	}

	slide1 := contentXMLs["ppt/slides/slide1.xml"]
	if !contains(slide1, "Bullet 3") || !contains(slide1, "Bullet 4") {
		t.Errorf("Expected new bullets in slide1, got: %s", slide1)
	}
	// Original bullets should still exist
	if !contains(slide1, "Bullet 1") || !contains(slide1, "Bullet 2") {
		t.Errorf("Original bullets should be preserved")
	}

	// Test: Empty bullets (no-op)
	modified2, err := AppendSlideBullets(pptx, 1, []string{})
	if err != nil {
		t.Fatalf("AppendSlideBullets with empty list failed: %v", err)
	}
	if len(modified2) != len(pptx) {
		t.Error("Empty bullets should return unchanged PPTX")
	}
}

func TestReplaceBodyParagraph(t *testing.T) {
	pptx := createMinimalPPTX(t)

	// Test: Replace first paragraph in slide 1 body
	modified, err := ReplaceBodyParagraph(pptx, 1, 1, "Replaced Paragraph")
	if err != nil {
		t.Fatalf("ReplaceBodyParagraph failed: %v", err)
	}

	contentXMLs, err := ExtractContentXML(modified, FileTypePPTX)
	if err != nil {
		t.Fatalf("ExtractContentXML failed: %v", err)
	}

	slide1 := contentXMLs["ppt/slides/slide1.xml"]
	if !contains(slide1, "Replaced Paragraph") {
		t.Errorf("Expected 'Replaced Paragraph' in slide1")
	}

	// Test: Invalid paragraph index
	_, err = ReplaceBodyParagraph(pptx, 1, 999, "Test")
	if err == nil {
		t.Error("Expected error for non-existent paragraph")
	}

	// Test: paragraphIndex < 1
	_, err = ReplaceBodyParagraph(pptx, 1, 0, "Test")
	if err == nil {
		t.Error("Expected error for paragraphIndex < 1")
	}
}

func TestUpdateTableCells(t *testing.T) {
	pptx := createMinimalPPTX(t)

	// Test: Update cell in slide 2 table
	updates := []CellUpdate{
		{Row: 1, Col: 1, Value: "Updated A1"},
	}
	modified, err := UpdateTableCells(pptx, 2, updates)
	if err != nil {
		t.Fatalf("UpdateTableCells failed: %v", err)
	}

	contentXMLs, err := ExtractContentXML(modified, FileTypePPTX)
	if err != nil {
		t.Fatalf("ExtractContentXML failed: %v", err)
	}

	slide2 := contentXMLs["ppt/slides/slide2.xml"]
	if !contains(slide2, "Updated A1") {
		t.Errorf("Expected 'Updated A1' in slide2 table")
	}

	// Test: Empty updates (no-op)
	modified2, err := UpdateTableCells(pptx, 2, []CellUpdate{})
	if err != nil {
		t.Fatalf("UpdateTableCells with empty list failed: %v", err)
	}
	if len(modified2) != len(pptx) {
		t.Error("Empty updates should return unchanged PPTX")
	}

	// Test: Invalid row/col
	_, err = UpdateTableCells(pptx, 2, []CellUpdate{{Row: 0, Col: 1, Value: "Test"}})
	if err == nil {
		t.Error("Expected error for row < 1")
	}
}

func TestReplaceSlideXML(t *testing.T) {
	pptx := createMinimalPPTX(t)

	newSlideXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld>
    <p:spTree>
      <p:sp>
        <p:txBody>
          <a:p><a:r><a:t>Dark Slide Title</a:t></a:r></a:p>
          <a:p><a:r><a:t>Dark style body</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
    </p:spTree>
  </p:cSld>
</p:sld>`

	modified, err := ReplaceSlideXML(pptx, 2, newSlideXML)
	if err != nil {
		t.Fatalf("ReplaceSlideXML failed: %v", err)
	}

	contentXMLs, err := ExtractContentXML(modified, FileTypePPTX)
	if err != nil {
		t.Fatalf("ExtractContentXML failed: %v", err)
	}

	slide2 := contentXMLs["ppt/slides/slide2.xml"]
	if !contains(slide2, "Dark Slide Title") || !contains(slide2, "Dark style body") {
		t.Fatalf("expected slide2 to be replaced, got: %s", slide2)
	}
	if contains(slide2, "Cell A1") {
		t.Fatalf("old slide2 content should be replaced")
	}
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// createTestPPTX creates a minimal PPTX ZIP from content XMLs
func createTestPPTX(contentXMLs map[string]string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	// Required minimal files
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
  <Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>
  <Override PartName="/ppt/slides/slide2.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>
</Relationships>`,
		"ppt/presentation.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <p:sldIdLst>
    <p:sldId id="256" r:id="rId1"/>
    <p:sldId id="257" r:id="rId2"/>
  </p:sldIdLst>
</p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide2.xml"/>
</Relationships>`,
	}

	// Merge in content XMLs
	for path, content := range contentXMLs {
		files[path] = content
	}

	for name, content := range files {
		f, _ := w.Create(name)
		f.Write([]byte(content)) //nolint:errcheck
	}

	w.Close()
	return buf.Bytes()
}
