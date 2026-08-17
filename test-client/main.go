package main

import (
	"fmt"
	"log"
	"net/http"
)

// baseHandler responds to the client with a simple message
func baseHandler(w http.ResponseWriter, r *http.Request) {
	// Write a plain text response to the client
	fmt.Println("GET Request Received")
	const response = `{"message": "ok"}`
	fmt.Fprint(w, response)
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	// Write a plain text response to the client
	fmt.Println("Hello Request Received")
	const response = `{"message": "hello"}`
	fmt.Fprint(w, response)
}


func main() {
	// Create a new request multiplexer (router)
	mux := http.NewServeMux()

	// Register the route pattern and its corresponding handler
	mux.HandleFunc("/", baseHandler)
	mux.HandleFunc("/hello", helloHandler)

	// Define the network port to listen on
	port := ":3000"
	fmt.Printf("Server is starting on http://localhost%s\n", port)

	// Start the server and listen for incoming TCP connections
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
