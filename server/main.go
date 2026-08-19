package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"tunnelforge/internal/proto"
	"tunnelforge/server/utils"

	"github.com/gin-gonic/gin"
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

	go func() {
		for {
		conn, err := listner.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: " + err.Error())
			continue
		}
		
		go handleAgent(conn)
		}
	}()
	
	r := gin.Default()

	r.Any("/test/:subdomain", proxyHandler)
	r.Any("/test/:subdomain/*path", proxyHandler)

	fmt.Println("Agent server listening on :" + PORT)
	fmt.Println("HTTP server listening on :8000")

	if err := r.Run(":8000"); err != nil {
		panic(err)
	}
}

func proxyHandler(c *gin.Context) {
	subdomain := c.Param("subdomain")

	fmt.Println("Incoming Request for: ", subdomain)

	session, ok := agentSession.Load(subdomain)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "agent is not connected",
		})
		return
	}

	stream, err := session.Open()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "failed to open tunnel stream",
		})
		return
	}
	defer stream.Close()

	fmt.Println("Opened yamux stream for:", subdomain)

	if err := c.Request.Write(stream); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "failed to write request to agent",
		})
		return
	}
	
	fmt.Println("Request forwarded to agent")

	response, err := http.ReadResponse(
		bufio.NewReader(stream),
		c.Request,
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "failed to read response from agent",
		})
		return
	}

	defer response.Body.Close()

	for key, values := range response.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	c.Status(response.StatusCode)

	if _, err := io.Copy(c.Writer, response.Body); err != nil {
		fmt.Println("Error copying response body:", err)
	}
}