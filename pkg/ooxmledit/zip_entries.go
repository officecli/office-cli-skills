package ooxmledit

import (
	"archive/zip"
	"bytes"
	"fmt"
)

// ZipEntries opens ooxmlBytes as a ZIP archive and returns the reader's file list.
// This is the canonical zip-walk entry point for the ooxmledit package;
// all code that needs to iterate over ZIP entries should use this function
// rather than calling zip.NewReader directly.
func ZipEntries(ooxmlBytes []byte) ([]*zip.File, error) {
	reader, err := zip.NewReader(bytes.NewReader(ooxmlBytes), int64(len(ooxmlBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to open OOXML zip: %w", err)
	}
	return reader.File, nil
}
