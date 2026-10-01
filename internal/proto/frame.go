package proto

import (
	"fmt"
	"io"
)

// WriteStreamHeader writes a binary frame header to identify which
// subdomain a Yamux data stream belongs to.
//
// Wire format:
//
//	[1 byte: subdomain length (uint8)] [N bytes: subdomain UTF-8]
//
// Max subdomain length is 63 bytes (DNS label limit).
func WriteStreamHeader(w io.Writer, subdomain string) error {
	n := len(subdomain)
	if n == 0 {
		return fmt.Errorf("subdomain must not be empty")
	}
	if n > 63 {
		return fmt.Errorf("subdomain too long: %d bytes (max 63)", n)
	}

	header := make([]byte, 1+n)
	header[0] = byte(n)
	copy(header[1:], subdomain)

	_, err := w.Write(header)
	if err != nil {
		return fmt.Errorf("writing stream header: %w", err)
	}

	return nil
}

// ReadStreamHeader reads the binary frame header from a Yamux data
// stream and returns the subdomain it targets.
//
// Header Format:
//
//	[1 byte: subdomain length (uint8)] [N bytes: subdomain UTF-8]
//
// Max subdomain length is 63 bytes (DNS label limit).
func ReadStreamHeader(r io.Reader) (string, error) {
	lenBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return "", fmt.Errorf("reading subdomain length: %w", err)
	}

	n := int(lenBuf[0])
	if n == 0 {
		return "", fmt.Errorf("subdomain length is zero")
	}

	subBuf := make([]byte, n)
	if _, err := io.ReadFull(r, subBuf); err != nil {
		return "", fmt.Errorf("reading subdomain: %w", err)
	}

	return string(subBuf), nil
}
