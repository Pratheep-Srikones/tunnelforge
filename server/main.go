package main

import (
	"fmt"
	"net"
	"tunnelforge/server/tunnel"

	"github.com/gin-gonic/gin"
)

const PORT = "7000"

var registry = tunnel.NewAgentRegistry()

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

	r.Any("/*path", proxyHandler)

	fmt.Println("Agent server listening on :" + PORT)
	fmt.Println("HTTP server listening on :8000")

	if err := r.Run(":8000"); err != nil {
		panic(err)
	}
}
