package util

import "bytes"

type BoundedBuffer struct {
	buf       bytes.Buffer
	Limit     int64
	TotalRead int64
}

func (b *BoundedBuffer) Write(p []byte) (n int, err error) {
	b.TotalRead += int64(len(p))
	if b.Limit > 0 && int64(b.buf.Len()) < b.Limit {
		remaining := b.Limit - int64(b.buf.Len())
		if int64(len(p)) <= remaining {
			b.buf.Write(p)
		} else { // truncate up to limit
			b.buf.Write(p[:remaining])
		}
	}
	return len(p), nil
}

func (b *BoundedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

func (b *BoundedBuffer) Read() []byte {
	return b.Bytes()
}
