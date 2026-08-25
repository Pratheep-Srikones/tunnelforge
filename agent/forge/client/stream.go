package client

import (
	"fmt"
	"io"
	"net"
	"tunnelforge/internal/proto"
)

func (c *Client) handleStream(stream net.Conn) {
	if err := c.forwardStream(stream); err != nil {
		fmt.Println("Stream error:", err)
	}
}

func (c *Client) forwardStream(stream net.Conn) error {
	defer stream.Close()

	// Read the binary stream header to determine which subdomain
	// this stream is intended for.
	subdomain, err := proto.ReadStreamHeader(stream)
	if err != nil {
		return fmt.Errorf("reading stream header: %w", err)
	}

	localAddr, ok := c.Tunnels[subdomain]
	if !ok {
		return fmt.Errorf("unknown subdomain %q in stream header", subdomain)
	}

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

	fmt.Printf("Routing %s → %s\n", subdomain, localAddr)

	errCh := make(chan error, 2)

	go copyStream(
		errCh,
		stream,
		localConn,
	)

	go copyStream(
		errCh,
		localConn,
		stream,
	)

	err = <-errCh

	if err != nil && err != io.EOF {
		return err
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
