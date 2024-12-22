package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
	"github.com/MegaGrindStone/go-mcp/pkg/servers/filesystem"
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

	srv, err := filesystem.NewStdIOServer(*path)
	if err != nil {
		fmt.Println("Error: failed to create filesystem server:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newClient(ctx)
	cli := mcp.NewStdIOClient(c, srv.MCPServer)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		cancel()
	}()

	readyChan := make(chan struct{})
	errsChan := make(chan error)
	go func() {
		err := cli.Run(ctx, c, c, readyChan, errsChan)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Print(err)
			}
		}
	}()
	go func() {
		for err := range errsChan {
			fmt.Print(err)
		}
	}()

	fmt.Println("Initializing session...")
	<-readyChan
	fmt.Println("Session initialized!")

	c.run()
	fmt.Println("Exiting...")
}
