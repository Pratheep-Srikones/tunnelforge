package client

import (
	"fmt"
	"io"
	"net"
)

func (c *Client) handleStream(stream net.Conn) {
	if err := c.forwardStream(stream); err != nil {
		fmt.Println("Stream error:", err)
	}
}

func (c *Client) forwardStream(stream net.Conn) error {
	defer stream.Close()

	localConn, err := net.Dial("tcp", c.LocalAddr)
	if err != nil {
		return fmt.Errorf(
			"connecting to local service %s: %w",
			c.LocalAddr,
			err,
		)
	}

	defer localConn.Close()

	fmt.Println(
		"Connected to local service:",
		c.LocalAddr,
	)

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