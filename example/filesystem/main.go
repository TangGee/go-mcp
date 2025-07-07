package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/TangGee/go-mcp"
	"github.com/TangGee/go-mcp/servers/filesystem"
)

func main() {
	path := flag.String("path", "", "Path to process (required)")
	flag.StringVar(path, "p", "", "Path to process (required) (shorthand)")

	flag.Parse()

	if *path == "" {
		fmt.Println("Error: path is required")
		flag.Usage()
		os.Exit(1)
	}

	srvReader, srvWriter, err := os.Pipe()
	if err != nil {
		fmt.Println("Error: failed to create pipes:", err)
		os.Exit(1)
	}

	cliReader, cliWriter, err := os.Pipe()
	if err != nil {
		fmt.Println("Error: failed to create pipes:", err)
		os.Exit(1)
	}

	cliIO := mcp.NewStdIO(cliReader, srvWriter)
	srvIO := mcp.NewStdIO(srvReader, cliWriter)

	server, err := filesystem.NewServer([]string{*path})
	if err != nil {
		fmt.Println("Error: failed to create filesystem server:", err)
		os.Exit(1)
	}

	srv := mcp.NewServer(mcp.Info{
		Name:    "filesystem",
		Version: "1.0",
	}, srvIO,
		mcp.WithServerPingInterval(10*time.Second),
		mcp.WithServerPingTimeout(5*time.Second),
		mcp.WithToolServer(server),
	)

	go srv.Serve()

	cli := newClient(cliIO)
	go cli.run(*path)

	<-cli.done

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		fmt.Printf("Server forced to shutdown: %v", err)
		return
	}

	err = cli.cli.Disconnect(shutdownCtx)
	if err != nil {
		fmt.Printf("Client forced to shutdown: %v", err)
		return
	}
}
