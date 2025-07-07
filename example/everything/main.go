package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/TangGee/go-mcp"
	"github.com/TangGee/go-mcp/servers/everything"
)

var port = "8080"

func main() {
	msgURL := fmt.Sprintf("%s/message", baseURL())
	sse := mcp.NewSSEServer(msgURL)
	server := everything.NewServer()

	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		ReadHeaderTimeout: 15 * time.Second,
	}

	http.Handle("/sse", sse.HandleSSE())
	http.Handle("/message", sse.HandleMessage())

	go func() {
		fmt.Printf("Server starting on %s\n", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	srv := mcp.NewServer(mcp.Info{
		Name:    "everything",
		Version: "1.0",
	}, sse,
		mcp.WithServerPingInterval(10*time.Second),
		mcp.WithServerPingTimeout(5*time.Second),
		mcp.WithPromptServer(server),
		mcp.WithResourceServer(server),
		mcp.WithToolServer(server),
		mcp.WithResourceSubscriptionHandler(server),
		mcp.WithLogHandler(server),
	)

	go srv.Serve()

	// Wait for the server to start
	time.Sleep(time.Second)
	fmt.Println("Server started")

	cli := newClient()
	go cli.run()

	<-cli.done

	fmt.Println("Client requested shutdown...")
	fmt.Println("Shutting down server...")
	server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := cli.cli.Disconnect(ctx); err != nil {
		fmt.Printf("Client forced to shutdown: %v", err)
		return
	}
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("Server forced to shutdown: %v", err)
		return
	}
	if err := httpSrv.Shutdown(ctx); err != nil {
		fmt.Printf("Server forced to shutdown: %v", err)
		return
	}

	fmt.Println("Server exited gracefully")
}

func baseURL() string {
	return fmt.Sprintf("http://localhost:%s", port)
}
