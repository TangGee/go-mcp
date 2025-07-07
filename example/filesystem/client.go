package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TangGee/go-mcp"
	"github.com/TangGee/go-mcp/servers/filesystem"
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
	}, transport,
		mcp.WithClientPingInterval(10*time.Second),
		mcp.WithClientPingTimeout(5*time.Second),
	)

	return client{
		cli:       cli,
		ctx:       ctx,
		cancel:    cancel,
		closeLock: new(sync.Mutex),
		done:      make(chan struct{}),
	}
}

func (c client) run(rootPath string) {
	defer c.stop()

	if err := c.cli.Connect(c.ctx); err != nil {
		fmt.Printf("failed to connect to server: %v\n", err)
		return
	}
	go c.listenInterruptSignal()

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
			if c.callTool(tool, rootPath) {
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

func (c client) callTool(tool mcp.Tool, rootPath string) bool {
	switch tool.Name {
	case "read_file":
		return c.callReadFile(rootPath)
	case "read_multiple_files":
		return c.callReadMultipleFiles(rootPath)
	case "write_file":
		return c.callWriteFile(rootPath)
	case "edit_file":
		return c.callEditFile(rootPath)
	case "create_directory":
		return c.callCreateDirectory(rootPath)
	case "list_directory":
		return c.callListDirectory(rootPath)
	case "directory_tree":
		return c.callDirectoryTree(rootPath)
	case "move_file":
		return c.callMoveFile(rootPath)
	case "search_files":
		return c.callSearchFiles(rootPath)
	case "get_file_info":
		return c.callGetFileInfo(rootPath)
	case "list_allowed_directories":
		return c.listAllowedDirectories()
	}
	return false
}

func (c client) callReadFile(rootPath string) bool {
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
		Path: filepath.Join(rootPath, input),
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

func (c client) callReadMultipleFiles(rootPath string) bool {
	fmt.Println("Enter relative path (from the root) to the files (comma-separated):")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	inputArr := strings.Split(input, ",")
	var paths []string
	for _, path := range inputArr {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		paths = append(paths, filepath.Join(rootPath, path))
	}

	args := filesystem.ReadMultipleFilesArgs{
		Paths: paths,
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

func (c client) callWriteFile(rootPath string) bool {
	fmt.Println("Enter relative path (from the root) to the file:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	path := filepath.Join(rootPath, input)

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

func (c client) callEditFile(rootPath string) bool {
	fmt.Println("Enter relative path (from the root) to the file:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	path := filepath.Join(rootPath, input)

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

	if len(edits) == 0 {
		fmt.Println("No edits provided")
		return false
	}

	fmt.Println("Dry run? (y/n)")

	input, err = c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	dryRun := input == "y"

	args := filesystem.EditFileArgs{
		Path:   path,
		Edits:  edits,
		DryRun: dryRun,
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

func (c client) callCreateDirectory(rootPath string) bool {
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
		Path: filepath.Join(rootPath, input),
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

func (c *client) callListDirectory(rootPath string) bool {
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
		Path: filepath.Join(rootPath, input),
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

func (c client) callDirectoryTree(rootPath string) bool {
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
		Path: filepath.Join(rootPath, input),
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

func (c client) callMoveFile(rootPath string) bool {
	fmt.Println("Enter relative path (from the root) to the source file:")

	input, err := c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	path := filepath.Join(rootPath, input)

	fmt.Println("Enter relative path (from the root) to the destination file:")

	input, err = c.waitStdIOInput()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	destination := filepath.Join(rootPath, input)

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

func (c client) callSearchFiles(rootPath string) bool {
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
		Path:    rootPath,
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

func (c client) callGetFileInfo(rootPath string) bool {
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
		Path: filepath.Join(rootPath, input),
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

func (c client) listAllowedDirectories() bool {
	params := mcp.CallToolParams{
		Name: "list_allowed_directories",
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

	for _, dir := range result.Content {
		fmt.Println(dir.Text)
	}

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
