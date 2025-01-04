package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/MegaGrindStone/go-mcp"
	"github.com/MegaGrindStone/go-mcp/servers/everything"
)

var port = "8080"

func main() {
	msgURL := fmt.Sprintf("%s/message", baseURL())
	sse := mcp.NewSSEServer(msgURL)
	server := everything.NewServer()

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		ReadHeaderTimeout: 15 * time.Second,
	}

	http.Handle("/sse", sse.HandleSSE())
	http.Handle("/message", sse.HandleMessage())

	go func() {
		fmt.Printf("Server starting on %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	srvCtx, srvCancel := context.WithCancel(context.Background())

	go mcp.Serve(srvCtx, server, sse,
		mcp.WithServerPingInterval(30*time.Second),
		mcp.WithPromptServer(server),
		mcp.WithResourceServer(server),
		mcp.WithToolServer(server),
		mcp.WithResourceSubscriptionHandler(server),
		mcp.WithLogHandler(server),
	)

	// Wait for the server to start
	time.Sleep(time.Second)
	fmt.Println("Server started")

	cli := newClient()
	go cli.run()

	<-cli.done

	fmt.Println("Client requested shutdown...")
	fmt.Println("Shutting down server...")
	srvCancel()
	server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("Server forced to shutdown: %v", err)
		return
	}

	fmt.Println("Server exited gracefully")
}

func baseURL() string {
	return fmt.Sprintf("http://localhost:%s", port)
}
