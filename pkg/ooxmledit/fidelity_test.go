package ooxmledit

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestFidelityInventory(t *testing.T) {
	data := buildTestZip(t, map[string][]byte{
		"ppt/slides/slide1.xml":    []byte("<slide>text</slide>"),
		"ppt/media/image1.png":     []byte("fake-png-data"),
		"ppt/media/image2.jpg":     []byte("fake-jpg-data"),
		"ppt/charts/chart1.xml":    []byte("<chart>data</chart>"),
		"ppt/embeddings/obj1.bin":  []byte("embedded-object"),
		"ppt/diagrams/data1.xml":   []byte("<dgm>smartart</dgm>"),
		"ppt/styles.xml":           []byte("<styles/>"),
	})

	inv, err := InventoryNonText(data)
	if err != nil {
		t.Fatalf("InventoryNonText: %v", err)
	}

	if len(inv.Media) != 2 {
		t.Errorf("Media count = %d, want 2", len(inv.Media))
	}
	if _, ok := inv.Media["ppt/media/image1.png"]; !ok {
		t.Error("missing ppt/media/image1.png in Media")
	}
	if _, ok := inv.Media["ppt/media/image2.jpg"]; !ok {
		t.Error("missing ppt/media/image2.jpg in Media")
	}
	if len(inv.Charts) != 1 {
		t.Errorf("Charts count = %d, want 1", len(inv.Charts))
	}
	if len(inv.Embeddings) != 1 {
		t.Errorf("Embeddings count = %d, want 1", len(inv.Embeddings))
	}
	if len(inv.SmartArt) != 1 {
		t.Errorf("SmartArt count = %d, want 1", len(inv.SmartArt))
	}
	if len(inv.CustomStyles) != 1 {
		t.Errorf("CustomStyles count = %d, want 1", len(inv.CustomStyles))
	}
}

func TestFidelityDiffDetectsDropped(t *testing.T) {
	beforeData := buildTestZip(t, map[string][]byte{
		"ppt/media/image1.png":  []byte("image-data-1"),
		"ppt/media/image2.jpg":  []byte("image-data-2"),
		"ppt/charts/chart1.xml": []byte("<chart>v1</chart>"),
	})
	afterData := buildTestZip(t, map[string][]byte{
		"ppt/media/image1.png": []byte("image-data-1"),
	})

	before, err := InventoryNonText(beforeData)
	if err != nil {
		t.Fatalf("InventoryNonText(before): %v", err)
	}
	after, err := InventoryNonText(afterData)
	if err != nil {
		t.Fatalf("InventoryNonText(after): %v", err)
	}

	preserved, dropped := DiffInventory(before, after)

	if len(preserved) != 1 {
		t.Errorf("preserved count = %d, want 1", len(preserved))
	}
	if len(dropped) != 2 {
		t.Errorf("dropped count = %d, want 2", len(dropped))
	}

	droppedSet := make(map[string]bool)
	for _, d := range dropped {
		droppedSet[d] = true
	}
	if !droppedSet["ppt/media/image2.jpg"] {
		t.Error("expected ppt/media/image2.jpg in dropped")
	}
	if !droppedSet["ppt/charts/chart1.xml"] {
		t.Error("expected ppt/charts/chart1.xml in dropped")
	}
}

func TestFidelityHashesUncompressedPayload(t *testing.T) {
	payload := []byte("identical-uncompressed-content-for-both-zips")

	zip1 := buildTestZipWithMethod(t, map[string][]byte{
		"ppt/media/image1.png": payload,
	}, zip.Deflate)

	zip2 := buildTestZipWithMethod(t, map[string][]byte{
		"ppt/media/image1.png": payload,
	}, zip.Store)

	inv1, err := InventoryNonText(zip1)
	if err != nil {
		t.Fatalf("InventoryNonText(deflate): %v", err)
	}
	inv2, err := InventoryNonText(zip2)
	if err != nil {
		t.Fatalf("InventoryNonText(store): %v", err)
	}

	hash1 := inv1.Media["ppt/media/image1.png"]
	hash2 := inv2.Media["ppt/media/image1.png"]

	if hash1 == "" || hash2 == "" {
		t.Fatal("expected non-empty hashes")
	}
	if hash1 != hash2 {
		t.Errorf("hashes differ for same uncompressed content:\n  deflate: %s\n  store:   %s", hash1, hash2)
	}

	if !bytes.Equal(zip1, zip2) {
		// Sanity: the raw zip bytes themselves should differ (different compression methods)
		// but the uncompressed-payload hashes should match.
	} else {
		t.Log("note: raw zip bytes happen to be equal (unexpected but not a test failure)")
	}
}

func TestFidelityDiffDetectsHashChange(t *testing.T) {
	beforeData := buildTestZip(t, map[string][]byte{
		"ppt/media/image1.png": []byte("original-content"),
	})
	afterData := buildTestZip(t, map[string][]byte{
		"ppt/media/image1.png": []byte("modified-content"),
	})

	before, err := InventoryNonText(beforeData)
	if err != nil {
		t.Fatalf("InventoryNonText(before): %v", err)
	}
	after, err := InventoryNonText(afterData)
	if err != nil {
		t.Fatalf("InventoryNonText(after): %v", err)
	}

	preserved, dropped := DiffInventory(before, after)

	if len(preserved) != 0 {
		t.Errorf("preserved count = %d, want 0", len(preserved))
	}
	if len(dropped) != 1 || dropped[0] != "ppt/media/image1.png" {
		t.Errorf("dropped = %v, want [ppt/media/image1.png]", dropped)
	}
}

// buildTestZip creates a zip archive from a map of path->content using Deflate.
func buildTestZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	return buildTestZipWithMethod(t, entries, zip.Deflate)
}

// buildTestZipWithMethod creates a zip archive with the specified compression method.
func buildTestZipWithMethod(t *testing.T, entries map[string][]byte, method uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range entries {
		header := &zip.FileHeader{
			Name:   name,
			Method: method,
		}
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
