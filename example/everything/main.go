package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
	"github.com/MegaGrindStone/go-mcp/pkg/servers/everything"
)

var port = "8080"

func main() {
	sse := everything.NewSSEServer(mcp.WithServerPingInterval(30 * time.Second))

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		ReadHeaderTimeout: 15 * time.Second,
	}

	msgBaseURL := fmt.Sprintf("%s/message", baseURL())
	http.Handle("/sse", sse.MCPServer.HandleSSE(msgBaseURL))
	http.Handle("/message", sse.MCPServer.HandleMessage())

	go func() {
		fmt.Printf("Server starting on %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for the server to start
	time.Sleep(time.Second)
	fmt.Println("Server started")

	cli := newClient()
	go func() {
		cli.run()
	}()

	<-cli.done

	fmt.Println("Client requested shutdown...")
	fmt.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	sse.MCPServer.Stop()
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("Server forced to shutdown: %v", err)
		return
	}

	fmt.Println("Server exited gracefully")
}

func baseURL() string {
	return fmt.Sprintf("http://localhost:%s", port)
}
