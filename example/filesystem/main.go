package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/MegaGrindStone/go-mcp"
	"github.com/MegaGrindStone/go-mcp/servers/filesystem"
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

	srvReader, srvWriter := io.Pipe()
	cliReader, cliWriter := io.Pipe()

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
		mcp.WithServerPingInterval(30*time.Second),
		mcp.WithToolServer(server),
	)

	go srv.Serve()

	cli := newClient(cliIO)
	go cli.run(*path)

	<-cli.done

	if err := srv.Shutdown(context.Background()); err != nil {
		fmt.Printf("Server forced to shutdown: %v", err)
		return
	}
}
