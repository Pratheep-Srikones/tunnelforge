package util

import (
	"bytes"
	"testing"
)

func TestBoundedBuffer_UnderLimit(t *testing.T) {
	bb := &BoundedBuffer{Limit: 100}
	data := []byte("hello world")

	n, err := bb.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("expected written %d bytes, got %d", len(data), n)
	}

	if bb.TotalRead != int64(len(data)) {
		t.Fatalf("expected TotalRead %d, got %d", len(data), bb.TotalRead)
	}

	if !bytes.Equal(bb.Bytes(), data) {
		t.Fatalf("expected buffer %q, got %q", string(data), string(bb.Bytes()))
	}
}

func TestBoundedBuffer_ExceedLimit(t *testing.T) {
	limit := int64(10)
	bb := &BoundedBuffer{Limit: limit}
	data := []byte("0123456789abcdefghij") // 20 bytes

	n, err := bb.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("expected Write to report full len %d, got %d", len(data), n)
	}

	if bb.TotalRead != 20 {
		t.Fatalf("expected TotalRead 20, got %d", bb.TotalRead)
	}

	if int64(len(bb.Bytes())) != limit {
		t.Fatalf("expected buffer length %d, got %d", limit, len(bb.Bytes()))
	}

	expected := []byte("0123456789")
	if !bytes.Equal(bb.Bytes(), expected) {
		t.Fatalf("expected %q, got %q", string(expected), string(bb.Bytes()))
	}
}

func TestBoundedBuffer_MultipleWrites(t *testing.T) {
	limit := int64(8)
	bb := &BoundedBuffer{Limit: limit}

	_, _ = bb.Write([]byte("1234")) // 4 bytes
	_, _ = bb.Write([]byte("56"))   // 2 bytes (total 6)
	_, _ = bb.Write([]byte("7890")) // 4 bytes (total 10)

	if bb.TotalRead != 10 {
		t.Fatalf("expected TotalRead 10, got %d", bb.TotalRead)
	}

	expected := []byte("12345678")
	if !bytes.Equal(bb.Bytes(), expected) {
		t.Fatalf("expected %q, got %q", string(expected), string(bb.Bytes()))
	}
}
