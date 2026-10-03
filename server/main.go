package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"tunnelforge/internal/certs"
	"tunnelforge/server/routes"
	"tunnelforge/server/tunnel"

	"github.com/gin-gonic/gin"
)

const PORT = "7000"

var registry = tunnel.NewAgentRegistry()

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
