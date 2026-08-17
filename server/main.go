package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"tunnelforge/internal/proto"
	"tunnelforge/server/utils"

	"github.com/hashicorp/yamux"
)

const PORT = "7000"

var agentSession = utils.NewThreadSafeSessionMap()

func handleAgent(conn net.Conn) {
    defer conn.Close()

    decoder := json.NewDecoder(conn)

    var req proto.HandshakeRequest

    if err := decoder.Decode(&req); err != nil {
        fmt.Println("Error decoding handshake:", err)
        return
    }

    if req.Type != "handshake" {
        fmt.Printf(
            "Error: Invalid handshake type, received %s\n",
            req.Type,
        )
        return
    }

    fmt.Println("Agent connected:")
    fmt.Println("  Subdomain:", req.Subdomain)
    fmt.Println("  Token:", req.Token)

    response := proto.HandshakeResponse{
        Type:    "handshake_ack",
        OK:      true,
        Message: "server accepted your handshake",
    }

    encoder := json.NewEncoder(conn)

    if err := encoder.Encode(response); err != nil {
        fmt.Println("Error sending handshake response:", err)
        return
    }

    fmt.Println("Registered tunnel:", req.Subdomain)

	session, err := yamux.Server(conn, nil)
	if err != nil {
		panic("Error creating yamux server: " + err.Error())
	}
	defer session.Close()

	agentSession.Store(req.Subdomain, session)

	stream, err := session.Open()
	if err != nil {
		fmt.Println("Error opening stream:", err)
		return
	}

	fmt.Println("Opened stream to agent")

	_, err = stream.Write([]byte("hello from server"))
	if err != nil {
		fmt.Println("Error writing to stream:", err)
		return
	}

	for {
		stream, err := session.Accept()
		if err != nil {
			fmt.Println("Yamux session closed:", err)

			agentSession.Delete(req.Subdomain)
			return
		}

		fmt.Println("New yamux stream from:", req.Subdomain)

		go handleStream(stream)
    }
}

func handleStream(stream net.Conn) {
	defer stream.Close()

	fmt.Println("Stream opened")

	// For now, just read whatever the agent sends.
	buf := make([]byte, 1024)

	for {
		n, err := stream.Read(buf)
		if err != nil {
			if err == io.EOF {
				fmt.Println("Stream closed")
			} else {
				fmt.Println("Stream error:", err)
			}
			return
		}

		fmt.Println("Stream received:", string(buf[:n]))
	}
}

func main() {
	listner, err := net.Listen("tcp", ":"+PORT)
	if err != nil {
		panic("Error listening on port: " + err.Error())
	}
	defer listner.Close()

	for {
		conn, err := listner.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: " + err.Error())
			continue
		}
		
		go handleAgent(conn)
	}
}