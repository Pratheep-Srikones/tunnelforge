package transport

import (
	"bytes"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSConn wraps a websocket connection to be used as a
// neet.Conn connection
type WSConn struct {
	*websocket.Conn
	readBuf bytes.Buffer
	writeMu sync.Mutex
}

// Compile-time check that WSConn implements net.Conn.
var _ net.Conn = (*WSConn)(nil)

// NewWSConn wraps an existing WebSocket connection as a net.Conn.
func NewWSConn(ws *websocket.Conn) *WSConn {
	return &WSConn{
		Conn: ws,
	}
}

// Read reads binary data from the WebSocket connection into b.
func (c *WSConn) Read(b []byte) (int, error) {
	for c.readBuf.Len() == 0 {
		msgType, data, err := c.Conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		if msgType == websocket.BinaryMessage {
			c.readBuf.Write(data)
			break
		}
	}
	return c.readBuf.Read(b)
}

// Write sends data as a binary WebSocket frame.
func (c *WSConn) Write(b []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	err := c.Conn.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// SetDeadline sets both read and write deadlines.
func (c *WSConn) SetDeadline(t time.Time) error {
	if err := c.Conn.SetReadDeadline(t); err != nil {
		return err
	}
	return c.Conn.SetWriteDeadline(t)
}
