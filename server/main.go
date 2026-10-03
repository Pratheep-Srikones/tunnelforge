package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"tunnelforge/internal/certs"
	"tunnelforge/internal/transport"
	"tunnelforge/server/routes"
	"tunnelforge/server/tunnel"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const PORT = "7000"

var registry = tunnel.NewAgentRegistry()

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func main() {
	certFile := flag.String("tls-cert", "", "Path to TLS certificate file")
	keyFile := flag.String("tls-key", "", "Path to TLS private key file")
	flag.Parse()

	tlsConfig, err := certs.LoadServerTLSConfig(*certFile, *keyFile)
	if err != nil {
		panic("[Server] TLS initialization failed: " + err.Error())
	}

	listener, err := tls.Listen("tcp", ":"+PORT, tlsConfig)
	if err != nil {
		panic("[Server] Error listening with TLS on port " + PORT + ": " + err.Error())
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Println("Error accepting connection: " + err.Error())
				continue
			}
			// spawn new goroutine to handle the connection
			go handleAgent(conn)
		}
	}()

	r := gin.Default()

	r.GET("/tunnel", func(ctx *gin.Context) {
		wss, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
		if err != nil {
			fmt.Println("[Server] WebSocket upgrade failed:", err)
			return
		}
		conn := transport.NewWSConn(wss)
		go handleAgent(conn)
	})

	internal := r.Group("/forge/internal")
	routes.UseAuthRoutes(internal)
	routes.UseInternalRoutes(internal, registry)

	r.NoRoute(proxyHandler)

	fmt.Println("[Server] Agent server listening on :" + PORT)
	fmt.Println("[Server] HTTP server listening on :8000")

	if err := r.Run(":8000"); err != nil {
		panic(err)
	}
}
