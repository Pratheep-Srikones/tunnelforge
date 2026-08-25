package proto

import (
	"bytes"
	"testing"
)

func TestStreamHeaderRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		subdomain string
	}{
		{name: "short subdomain", subdomain: "api"},
		{name: "typical subdomain", subdomain: "test-app"},
		{name: "long subdomain", subdomain: "my-very-long-subdomain-name-for-testing"},
		{name: "single char", subdomain: "a"},
		{name: "max length 63", subdomain: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			if err := WriteStreamHeader(&buf, tt.subdomain); err != nil {
				t.Fatalf("WriteStreamHeader() error = %v", err)
			}

			// Verify wire format: first byte is length
			raw := buf.Bytes()
			if int(raw[0]) != len(tt.subdomain) {
				t.Fatalf("expected length byte %d, got %d", len(tt.subdomain), raw[0])
			}
			if len(raw) != 1+len(tt.subdomain) {
				t.Fatalf("expected %d total bytes, got %d", 1+len(tt.subdomain), len(raw))
			}

			got, err := ReadStreamHeader(&buf)
			if err != nil {
				t.Fatalf("ReadStreamHeader() error = %v", err)
			}

			if got != tt.subdomain {
				t.Fatalf("expected subdomain %q, got %q", tt.subdomain, got)
			}
		})
	}
}

func TestWriteStreamHeaderErrors(t *testing.T) {
	var buf bytes.Buffer

	if err := WriteStreamHeader(&buf, ""); err == nil {
		t.Fatal("expected error for empty subdomain")
	}

	long := make([]byte, 64)
	for i := range long {
		long[i] = 'a'
	}
	if err := WriteStreamHeader(&buf, string(long)); err == nil {
		t.Fatal("expected error for subdomain > 63 bytes")
	}
}

func TestReadStreamHeaderErrors(t *testing.T) {
	// Empty reader
	var buf bytes.Buffer
	if _, err := ReadStreamHeader(&buf); err == nil {
		t.Fatal("expected error for empty reader")
	}

	// Length byte says 5 but only 2 bytes of data follow
	buf.Reset()
	buf.Write([]byte{5, 'a', 'b'})
	if _, err := ReadStreamHeader(&buf); err == nil {
		t.Fatal("expected error for truncated subdomain")
	}

	// Zero length byte
	buf.Reset()
	buf.Write([]byte{0})
	if _, err := ReadStreamHeader(&buf); err == nil {
		t.Fatal("expected error for zero length subdomain")
	}
}

func TestStreamHeaderWithTrailingData(t *testing.T) {
	// Verify that reading the header doesn't consume trailing HTTP data
	var buf bytes.Buffer

	if err := WriteStreamHeader(&buf, "test-app"); err != nil {
		t.Fatalf("WriteStreamHeader() error = %v", err)
	}

	httpData := []byte("GET / HTTP/1.1\r\nHost: test-app.example.com\r\n\r\n")
	buf.Write(httpData)

	subdomain, err := ReadStreamHeader(&buf)
	if err != nil {
		t.Fatalf("ReadStreamHeader() error = %v", err)
	}

	if subdomain != "test-app" {
		t.Fatalf("expected subdomain %q, got %q", "test-app", subdomain)
	}

	// Remaining bytes should be the HTTP data
	remaining := buf.Bytes()
	if !bytes.Equal(remaining, httpData) {
		t.Fatalf("trailing data corrupted: got %q", remaining)
	}
}
