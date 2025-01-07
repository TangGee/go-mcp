package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"

	"github.com/MegaGrindStone/go-mcp"
	"github.com/MegaGrindStone/go-mcp/servers/filesystem"
)

type client struct {
	cli    *mcp.Client
	ctx    context.Context
	cancel context.CancelFunc

	closeLock *sync.Mutex
	closed    bool
	done      chan struct{}
}

func newClient(transport mcp.ClientTransport) client {
	ctx, cancel := context.WithCancel(context.Background())

	cli := mcp.NewClient(mcp.Info{
		Name:    "fileserver-client",
		Version: "1.0",
	}, transport, mcp.ServerRequirement{
		ToolServer: true,
	})

	return client{
		cli:       cli,
		ctx:       ctx,
		cancel:    cancel,
		closeLock: new(sync.Mutex),
		done:      make(chan struct{}),
	}
}

func (c client) run() {
	defer c.stop()

	ready := make(chan struct{})

	go func() {
		if err := c.cli.Connect(c.ctx, ready); err != nil {
			fmt.Printf("failed to connect to server: %v\n", err)
			return
		}
	}()
	go c.listenInterruptSignal()
	<-ready

	for {
		tools, err := c.cli.ListTools(c.ctx, mcp.ListToolsParams{})
		if err != nil {
			fmt.Printf("failed to list tools: %v\n", err)
			return
		}

		fmt.Println()
		for i, tool := range tools.Tools {
			fmt.Printf("%d. %s\n", i+1, tool.Name)
		}
		fmt.Println()

		fmt.Println("Type one of the commands:")
		fmt.Println("- call <tool number>: Call the tool with the given number, eg. call 1")
		fmt.Println("- desc <tool number>: Show the description of the tool with the given number, eg. desc 1")
		fmt.Println("- exit: Exit the program")

		input, err := c.waitStdIOInput()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			fmt.Print(err)
			continue
		}

		arrInput := strings.Split(input, " ")
		if len(arrInput) == 1 {
			if arrInput[0] == "exit" {
				return
			}
			fmt.Printf("Unknown command: %s\n", input)
			continue
		}

		if len(arrInput) != 2 {
			fmt.Printf("Invalid command: %s\n", input)
		}

		toolNumber, err := strconv.Atoi(arrInput[1])
		if err != nil {
			fmt.Printf("Invalid command: %s\n", input)
			continue
		}
		if toolNumber < 1 || toolNumber > len(tools.Tools) {
			fmt.Printf("Tool with number %d not found\n", toolNumber)
			continue
		}

		tool := tools.Tools[toolNumber-1]

		switch arrInput[0] {
		case "call":
			if c.callTool(tool) {
				return
			}
		case "desc":
			fmt.Printf("Description for tool %s: %s\n", tool.Name, tool.Description)
		}

		fmt.Println("Press enter to continue...")

		_, err = c.waitStdIOInput()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			fmt.Print(err)
			continue
		}
	}
}

func (c client) callTool(tool mcp.Tool) bool {
	switch tool.Name {
	case "read_file":
		return c.callReadFile()
	case "read_multiple_files":
		return c.callReadMultipleFiles()
	case "write_file":
		return c.callWriteFile()
	case "edit_file":
		return c.callEditFile()
	case "create_directory":
		return c.callCreateDirectory()
	case "list_directory":
		return c.callListDirectory()
	case "directory_tree":
		return c.callDirectoryTree()
	case "move_file":
		return c.callMoveFile()
	case "search_files":
		return c.callSearchFiles()
	case "get_file_info":
		return c.callGetFileInfo()
	}
	return false
}

func (c client) callReadFile() bool {
	fmt.Println("Enter relative path (from the root) to the file:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	args := filesystem.ReadFileArgs{
		Path: input,
	}
	argsBs, _ := json.Marshal(args)

	params := mcp.CallToolParams{
		Name:      "read_file",
		Arguments: argsBs,
	}
	result, err := c.cli.CallTool(c.ctx, params)
	if err != nil {
		fmt.Printf("failed to call tool: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("File is empty")
		return false
	}

	fmt.Printf("File content of %s:\n%s\n", input, result.Content[0].Text)

	return false
}

func (c client) callReadMultipleFiles() bool {
	fmt.Println("Enter relative path (from the root) to the files (comma-separated):")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	args := filesystem.ReadMultipleFilesArgs{
		Paths: strings.Split(input, ","),
	}
	argsBs, _ := json.Marshal(args)

	params := mcp.CallToolParams{
		Name:      "read_multiple_files",
		Arguments: argsBs,
	}
	result, err := c.cli.CallTool(c.ctx, params)
	if err != nil {
		fmt.Printf("failed to call tool: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("Files are empty")
		return false
	}

	var contents []string
	for _, content := range result.Content {
		contents = append(contents, content.Text)
	}
	fmt.Println(strings.Join(contents, "---\n"))

	return false
}

func (c client) callWriteFile() bool {
	fmt.Println("Enter relative path (from the root) to the file:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	path := input

	fmt.Println("Enter content:")

	input, err = c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	content := input

	args := filesystem.WriteFileArgs{
		Path:    path,
		Content: content,
	}
	argsBs, _ := json.Marshal(args)

	params := mcp.CallToolParams{
		Name:      "write_file",
		Arguments: argsBs,
	}
	result, err := c.cli.CallTool(c.ctx, params)
	if err != nil {
		fmt.Printf("failed to call tool: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("File is empty")
		return false
	}

	fmt.Println(result.Content[0].Text)

	return false
}

func (c client) callEditFile() bool {
	fmt.Println("Enter relative path (from the root) to the file:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	path := input

	fmt.Println("Enter edits (old text:new text), each separated by a comma:")

	input, err = c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	editsStr := strings.Split(input, ",")
	var edits []filesystem.EditOperation
	for _, edit := range editsStr {
		edit = strings.TrimSpace(edit)
		if edit == "" {
			continue
		}

		arrEdit := strings.Split(edit, ":")
		if len(arrEdit) != 2 {
			fmt.Printf("Invalid edit: %s\n", edit)
			continue
		}

		oldText := strings.TrimSpace(arrEdit[0])
		newText := strings.TrimSpace(arrEdit[1])

		if oldText == "" || newText == "" {
			fmt.Printf("Invalid edit: %s\n", edit)
			continue
		}

		edits = append(edits, filesystem.EditOperation{
			OldText: oldText,
			NewText: newText,
		})
	}

	args := filesystem.EditFileArgs{
		Path:  path,
		Edits: edits,
		// Because the server doesn't support diff yet, dryRun is not supported yet.
		DryRun: false,
	}
	argsBs, _ := json.Marshal(args)

	params := mcp.CallToolParams{
		Name:      "edit_file",
		Arguments: argsBs,
	}
	result, err := c.cli.CallTool(c.ctx, params)
	if err != nil {
		fmt.Printf("failed to call tool: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("File is empty")
		return false
	}

	fmt.Println(result.Content[0].Text)

	return false
}

func (c client) callCreateDirectory() bool {
	fmt.Println("Enter relative path (from the root) to the directory:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	args := filesystem.CreateDirectoryArgs{
		Path: input,
	}
	argsBs, _ := json.Marshal(args)

	params := mcp.CallToolParams{
		Name:      "create_directory",
		Arguments: argsBs,
	}
	result, err := c.cli.CallTool(c.ctx, params)
	if err != nil {
		fmt.Printf("failed to call tool: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("Directory is empty")
		return false
	}

	fmt.Println(result.Content[0].Text)

	return false
}

func (c *client) callListDirectory() bool {
	fmt.Println("Enter relative path (from the root) to the directory:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	args := filesystem.ListDirectoryArgs{
		Path: input,
	}
	argsBs, _ := json.Marshal(args)

	params := mcp.CallToolParams{
		Name:      "list_directory",
		Arguments: argsBs,
	}
	result, err := c.cli.CallTool(c.ctx, params)
	if err != nil {
		fmt.Printf("failed to call tool: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("Directory is empty")
		return false
	}

	for _, content := range result.Content {
		fmt.Println(content.Text)
	}

	return false
}

func (c client) callDirectoryTree() bool {
	fmt.Println("Enter relative path (from the root) to the directory:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	args := filesystem.DirectoryTreeArgs{
		Path: input,
	}
	argsBs, _ := json.Marshal(args)

	params := mcp.CallToolParams{
		Name:      "directory_tree",
		Arguments: argsBs,
	}
	result, err := c.cli.CallTool(c.ctx, params)
	if err != nil {
		fmt.Printf("failed to call tool: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("Directory is empty")
		return false
	}

	for _, content := range result.Content {
		fmt.Println(content.Text)
	}

	return false
}

func (c client) callMoveFile() bool {
	fmt.Println("Enter relative path (from the root) to the source file:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	path := input

	fmt.Println("Enter relative path (from the root) to the destination file:")

	input, err = c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	destination := input

	args := filesystem.MoveFileArgs{
		Source:      path,
		Destination: destination,
	}
	argsBs, _ := json.Marshal(args)

	params := mcp.CallToolParams{
		Name:      "move_file",
		Arguments: argsBs,
	}
	result, err := c.cli.CallTool(c.ctx, params)
	if err != nil {
		fmt.Printf("failed to call tool: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("File is empty")
		return false
	}

	fmt.Println(result.Content[0].Text)

	return false
}

func (c client) callSearchFiles() bool {
	fmt.Println("Enter pattern:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	pattern := input

	fmt.Println("Enter exclude patterns (comma-separated):")

	input, err = c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	excludePatternsStr := strings.Split(input, ",")
	var excludePatterns []string
	for _, excludePattern := range excludePatternsStr {
		excludePattern = strings.TrimSpace(excludePattern)
		if excludePattern == "" {
			continue
		}
		excludePatterns = append(excludePatterns, excludePattern)
	}

	args := filesystem.SearchFilesArgs{
		Path:    pattern,
		Pattern: pattern,
		Exclude: excludePatterns,
	}
	argsBs, _ := json.Marshal(args)

	params := mcp.CallToolParams{
		Name:      "search_files",
		Arguments: argsBs,
	}
	result, err := c.cli.CallTool(c.ctx, params)
	if err != nil {
		fmt.Printf("failed to call tool: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("Directory is empty")
		return false
	}

	for _, content := range result.Content {
		fmt.Println(content.Text)
	}

	return false
}

func (c client) callGetFileInfo() bool {
	fmt.Println("Enter relative path (from the root) to the file:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	args := filesystem.GetFileInfoArgs{
		Path: input,
	}
	argsBs, _ := json.Marshal(args)

	params := mcp.CallToolParams{
		Name:      "get_file_info",
		Arguments: argsBs,
	}
	result, err := c.cli.CallTool(c.ctx, params)
	if err != nil {
		fmt.Printf("failed to call tool: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("File is empty")
		return false
	}

	fmt.Println(result.Content[0].Text)

	return false
}

func (c client) listenInterruptSignal() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
	c.stop()
}

func (c client) waitStdIOInput() (string, error) {
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
	case <-c.ctx.Done():
		return "", os.ErrClosed
	case <-c.done:
		return "", os.ErrClosed
	case err := <-errsChan:
		return "", err
	case input := <-inputChan:
		return input, nil
	}
}

func (c *client) stop() {
	c.closeLock.Lock()
	defer c.closeLock.Unlock()

	c.cancel()
	if !c.closed {
		close(c.done)
		c.closed = true
	}
}
