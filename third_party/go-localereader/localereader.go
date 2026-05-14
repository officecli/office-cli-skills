package localereader

import (
	"bytes"
	"io"
)

func NewReader(r io.Reader) io.Reader {
	return r
}

func UTF8(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, NewReader(bytes.NewReader(b))); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
