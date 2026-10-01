package client

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"time"
	"tunnelforge/agent/forge/capture"
	"tunnelforge/agent/forge/util"
	"tunnelforge/internal/proto"
)

const maxCaptureBytes = 1024 * 1024

type readerConn struct {
	net.Conn
	r io.Reader
}

func (c *readerConn) Read(p []byte) (n int, err error) {
	return c.r.Read(p)
}

type recorderReader struct {
	r      io.Reader
	record bytes.Buffer
}

func (rr *recorderReader) Read(p []byte) (n int, err error) {
	n, err = rr.r.Read(p)
	if n > 0 {
		rr.record.Write(p[:n])
	}
	return n, err
}

func (c *Client) handleStream(stream net.Conn) {
	if err := c.forward(stream); err != nil {
		fmt.Println("[Stream] Stream error:", err)
	}
}

func (c *Client) forward(stream net.Conn) error {
	/*
		forward stream to local service
		1. read stream header
		2. find local service
		3. connect to local service
		4. forward stream
	*/

	defer stream.Close()

	// Read the binary stream header to determine which subdomain
	// this stream is intended for.
	subdomain, err := proto.ReadStreamHeader(stream)
	if err != nil {
		return fmt.Errorf("reading stream header: %w", err)
	}

	tunnelEntry, ok := c.Tunnels[subdomain]
	if !ok {
		return fmt.Errorf("unknown subdomain %q in stream header", subdomain)
	}
	localAddr := tunnelEntry.Local

	// dial a tcp connection to local service
	localConn, err := net.Dial("tcp", localAddr)
	if err != nil {
		return fmt.Errorf(
			"connecting to local service %s for %s: %w",
			localAddr,
			subdomain,
			err,
		)
	}

	defer localConn.Close()

	fmt.Printf("[Stream] Routing %s -> %s (capture: %t)\n", subdomain, localAddr, tunnelEntry.Capture)

	if tunnelEntry.Capture && c.Capturer != nil {
		return c.forwardWithCapture(stream, localConn, subdomain)
	}

	return c.forwardRaw(stream, localConn)
}

func (c *Client) forwardWithCapture(
	stream net.Conn,
	localConn net.Conn,
	subdomain string,
) error {
	startTime := time.Now()

	recorder := &recorderReader{r: stream}
	reqReader := bufio.NewReader(recorder)
	req, err := http.ReadRequest(reqReader)
	if err != nil {
		fmt.Println("[Intercept] Failed to Read Request, Falling back to raw copy")
		combinedStream := &readerConn{
			Conn: stream,
			r:    io.MultiReader(bytes.NewReader(recorder.record.Bytes()), stream),
		}
		return c.forwardRaw(combinedStream, localConn)
	}

	// Tell local server not to wait for keep-alive requests on this connection
	req.Close = true

	// Setup bounded tap for request body (cap at 1MB)
	reqCapture := &util.BoundedBuffer{Limit: maxCaptureBytes}
	if req.Body != nil {
		req.Body = io.NopCloser(io.TeeReader(req.Body, reqCapture))
	}

	// Forward request to local connection
	if err := req.Write(localConn); err != nil {
		return fmt.Errorf("forwarding request to local service: %w", err)
	}

	respReader := bufio.NewReader(localConn)
	resp, err := http.ReadResponse(respReader, req)
	if err != nil {
		return fmt.Errorf("reading response from local service: %w", err)
	}
	defer resp.Body.Close()

	respCapture := &util.BoundedBuffer{Limit: maxCaptureBytes}
	resp.Body = io.NopCloser(io.TeeReader(resp.Body, respCapture))

	// Forward response back to client stream
	if err := resp.Write(stream); err != nil {
		return fmt.Errorf("writing response to client stream: %w", err)
	}

	duration := time.Since(startTime).Milliseconds()

	// Non-blocking push to the ring buffer
	go func() {
		truncated := reqCapture.TotalRead > maxCaptureBytes || respCapture.TotalRead > maxCaptureBytes

		urlStr := "/"
		if req.URL != nil {
			urlStr = req.URL.String()
			if urlStr == "" {
				urlStr = req.RequestURI
			}
		}

		entry := &capture.RequestEntry{
			ID:              fmt.Sprintf("%d-%06d", time.Now().UnixMilli(), rand.Intn(1000000)),
			Subdomain:       subdomain,
			Timestamp:       startTime,
			Method:          req.Method,
			URL:             urlStr,
			RequestHeaders:  req.Header.Clone(),
			RequestBody:     reqCapture.Read(),
			ResponseStatus:  resp.StatusCode,
			ResponseHeaders: resp.Header.Clone(),
			ResponseBody:    respCapture.Read(),
			DurationMS:      duration,
			Truncated:       truncated,
		}
		_ = c.Capturer.Push(subdomain, entry)
	}()

	return nil
}
func (c *Client) forwardRaw(stream net.Conn, localConn net.Conn) error {
	var err error
	errCh := make(chan error, 2)

	// copy from local to server
	go copyStream(
		errCh,
		stream,
		localConn,
	)

	// copy from server to local
	go copyStream(
		errCh,
		localConn,
		stream,
	)

	err = <-errCh

	if err != nil && err != io.EOF {
		return fmt.Errorf("copying stream: %w", err)
	}

	return nil
}

func copyStream(
	errCh chan<- error,
	dst net.Conn,
	src net.Conn,
) {
	_, err := io.Copy(dst, src)

	errCh <- err
}
