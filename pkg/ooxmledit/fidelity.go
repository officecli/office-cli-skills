package ooxmledit

import (
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
)

// NonTextInventory holds SHA-256 hashes of non-text entries in an OOXML zip,
// keyed by relative path within the archive.
type NonTextInventory struct {
	Media        map[string]string
	Charts       map[string]string
	Embeddings   map[string]string
	CustomStyles map[string]string
	SmartArt     map[string]string
}

// FidelityDiff reports entries that were present in a before-inventory but
// missing or hash-changed in an after-inventory.
type FidelityDiff struct {
	Dropped []string
}

// InventoryNonText walks an OOXML zip and hashes every non-text entry.
//
// Hashes are SHA-256 of the *uncompressed* entry payload (read via
// *zip.File.Open), NOT of the raw deflate-compressed bytes. This is required
// for fidelity semantics: copyZipFile in pkg/ooxmledit/ooxmledit.go uses
// writer.CreateHeader (not CreateRaw) and re-deflates entries on round-trip,
// so raw-byte hashes would always differ even when payloads are bit-identical.
func InventoryNonText(ooxmlBytes []byte) (*NonTextInventory, error) {
	files, err := ZipEntries(ooxmlBytes)
	if err != nil {
		return nil, err
	}

	inv := &NonTextInventory{
		Media:        make(map[string]string),
		Charts:       make(map[string]string),
		Embeddings:   make(map[string]string),
		CustomStyles: make(map[string]string),
		SmartArt:     make(map[string]string),
	}

	for _, f := range files {
		bucket := classifyEntry(f.Name)
		if bucket == "" {
			continue
		}

		h, err := hashUncompressed(f)
		if err != nil {
			return nil, fmt.Errorf("hashing %s: %w", f.Name, err)
		}

		switch bucket {
		case "media":
			inv.Media[f.Name] = h
		case "charts":
			inv.Charts[f.Name] = h
		case "embeddings":
			inv.Embeddings[f.Name] = h
		case "smartart":
			inv.SmartArt[f.Name] = h
		case "customstyles":
			inv.CustomStyles[f.Name] = h
		}
	}

	return inv, nil
}

// DiffInventory compares before and after inventories and returns lists of
// preserved and dropped entry paths. An entry is "dropped" if it existed in
// before but is missing or has a different hash in after.
func DiffInventory(before, after *NonTextInventory) (preserved, dropped []string) {
	diffMap := func(b, a map[string]string) {
		for path, hash := range b {
			if afterHash, ok := a[path]; ok && afterHash == hash {
				preserved = append(preserved, path)
			} else {
				dropped = append(dropped, path)
			}
		}
	}

	diffMap(before.Media, after.Media)
	diffMap(before.Charts, after.Charts)
	diffMap(before.Embeddings, after.Embeddings)
	diffMap(before.CustomStyles, after.CustomStyles)
	diffMap(before.SmartArt, after.SmartArt)

	return preserved, dropped
}

func classifyEntry(name string) string {
	lower := strings.ToLower(name)

	mediaPrefixes := []string{"ppt/media/", "word/media/", "xl/media/"}
	for _, p := range mediaPrefixes {
		if strings.HasPrefix(lower, p) {
			return "media"
		}
	}

	if strings.Contains(lower, "/charts/") && strings.HasSuffix(lower, ".xml") {
		return "charts"
	}

	if strings.Contains(lower, "/embeddings/") {
		return "embeddings"
	}

	if strings.Contains(lower, "/diagrams/") {
		return "smartart"
	}

	if strings.Contains(lower, "/styles/") || strings.HasSuffix(lower, "/styles.xml") {
		return "customstyles"
	}

	return ""
}

func hashUncompressed(f interface{ Open() (io.ReadCloser, error) }) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
