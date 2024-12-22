package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := server{}
	c := newClient(ctx)
	srv := mcp.NewStdIOServer(s, mcp.WithPromptServer(s))
	cli := mcp.NewStdIOClient(c, srv)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		fmt.Println("Exiting...")
		cancel()
	}()

	readyChan := make(chan struct{})
	errsChan := make(chan error)
	go func() {
		err := cli.Run(ctx, c, c, readyChan, errsChan)
		if err != nil {
			log.Print(err)
		}
	}()
	go func() {
		for err := range errsChan {
			fmt.Print(err)
		}
	}()

	time.Sleep(time.Second) // Give time for cli to run

	for {
		fmt.Println("Choose commands number:")
		cmds := buildCommands(cli)
		for i, cmd := range cmds {
			fmt.Printf("%d. %s\n", i+1, cmd)
		}

		input, err := waitStdIOInput(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			fmt.Print(err)
			continue
		}
		inputNumber, err := strconv.Atoi(input)
		if err != nil {
			fmt.Printf("Invalid input: %s\n", input)
			continue
		}
		inputIdx := inputNumber - 1
		if inputIdx < 0 || inputIdx >= len(cmds) {
			fmt.Printf("Invalid input: %s\n", input)
			continue
		}

		cmd := cmds[inputIdx]

		exit := false
		switch cmd {
		case "prompts":
			exit = c.prompts()
		case "exit":
			exit = true
		}

		if exit {
			fmt.Println("Exiting...")
			return
		}
	}
}

func buildCommands(cli *mcp.StdIOClient) []string {
	var cmds []string
	if cli.PromptsCommandsAvailable() {
		cmds = append(cmds, "prompts")
	}
	if cli.ResourcesCommandsAvailable() {
		cmds = append(cmds, "resources")
	}
	if cli.ToolsCommandsAvailable() {
		cmds = append(cmds, "tools")
	}
	cmds = append(cmds, "exit")

	return cmds
}

func waitStdIOInput(ctx context.Context) (string, error) {
	inputChan := make(chan string)
	errsChan := make(chan error)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			inputChan <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			errsChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errsChan:
		return "", err
	case input := <-inputChan:
		return input, nil
	}
}
