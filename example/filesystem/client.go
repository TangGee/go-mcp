package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
	"github.com/google/uuid"
)

type client struct {
	ctx context.Context

	inChan  chan mcp.JSONRPCMessage
	outChan chan mcp.JSONRPCMessage
}

func newClient(ctx context.Context) *client {
	return &client{
		ctx:     ctx,
		inChan:  make(chan mcp.JSONRPCMessage, 5),
		outChan: make(chan mcp.JSONRPCMessage, 5),
	}
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

func (c *client) Info() mcp.Info {
	return mcp.Info{
		Name:    "fileserver-client",
		Version: "1.0",
	}
}

func (c *client) RequirePromptServer() bool {
	return false
}

func (c *client) RequireResourceServer() bool {
	return false
}

func (c *client) RequireToolServer() bool {
	return true
}

func (c *client) Read(p []byte) (int, error) {
	var msg mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	case msg = <-c.inChan:
	}

	msgBs, _ := json.Marshal(msg)
	n := copy(p, msgBs)
	return n, nil
}

func (c *client) Write(p []byte) (int, error) {
	var msg mcp.JSONRPCMessage
	if err := json.Unmarshal(p, &msg); err != nil {
		return 0, err
	}

	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	case c.outChan <- msg:
	}

	return len(p), nil
}

func (c *client) run() {
	for {
		tools, err := c.listTools()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
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

		input, err := waitStdIOInput(c.ctx)
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

		_, err = waitStdIOInput(c.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			fmt.Print(err)
			continue
		}
	}
}

func (c *client) listTools() (mcp.ToolList, error) {
	params := mcp.ToolsListParams{}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsList,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return mcp.ToolList{}, c.ctx.Err()
	case out = <-c.outChan:
	}

	if out.Error != nil {
		return mcp.ToolList{}, fmt.Errorf("result error: %w", out.Error)
	}

	var result mcp.ToolList
	if err := json.Unmarshal(out.Result, &result); err != nil {
		return mcp.ToolList{}, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return result, nil
}

func (c *client) callTool(tool mcp.Tool) bool {
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

func (c *client) callReadFile() bool {
	fmt.Println("Enter relative path (from the root) to the file:")

	input, err := waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	params := mcp.ToolsCallParams{
		Name: "read_file",
		Arguments: map[string]any{
			"path": input,
		},
	}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsCall,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return true
	case out = <-c.outChan:
	}

	if out.Error != nil {
		fmt.Printf("result error: %v\n", out.Error)
		return false
	}

	var result mcp.ToolResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("File is empty")
		return false
	}

	fmt.Printf("File content of %s:\n%s\n", input, result.Content[0].Text)

	return false
}

func (c *client) callReadMultipleFiles() bool {
	fmt.Println("Enter relative path (from the root) to the files (comma-separated):")

	input, err := waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	params := mcp.ToolsCallParams{
		Name: "read_multiple_files",
		Arguments: map[string]any{
			"paths": strings.Split(input, ","),
		},
	}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsCall,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return true
	case out = <-c.outChan:
	}

	if out.Error != nil {
		fmt.Printf("result error: %v\n", out.Error)
		return false
	}

	var result mcp.ToolResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
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

//nolint:dupl // Avoid cleverness.
func (c *client) callWriteFile() bool {
	fmt.Println("Enter relative path (from the root) to the file:")

	input, err := waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	path := input

	fmt.Println("Enter content:")

	input, err = waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	content := input

	params := mcp.ToolsCallParams{
		Name: "write_file",
		Arguments: map[string]any{
			"path":    path,
			"content": content,
		},
	}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsCall,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return true
	case out = <-c.outChan:
	}

	if out.Error != nil {
		fmt.Printf("result error: %v\n", out.Error)
		return false
	}

	var result mcp.ToolResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("File is empty")
		return false
	}

	fmt.Println(result.Content[0].Text)

	return false
}

func (c *client) callEditFile() bool {
	fmt.Println("Enter relative path (from the root) to the file:")

	input, err := waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	path := input

	fmt.Println("Enter edits (old text:new text), each separated by a comma:")

	input, err = waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	editsStr := strings.Split(input, ",")
	var edits []any
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

		edits = append(edits, map[string]any{
			"oldText": oldText,
			"newText": newText,
		})
	}

	params := mcp.ToolsCallParams{
		Name: "edit_file",
		Arguments: map[string]any{
			"path":  path,
			"edits": edits,
			// Because the server doesn't support diff yet, dryRun is not supported yet.
			"dryRun": false,
		},
	}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsCall,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return true
	case out = <-c.outChan:
	}

	if out.Error != nil {
		fmt.Printf("result error: %v\n", out.Error)
		return false
	}

	var result mcp.ToolResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("File is empty")
		return false
	}

	fmt.Println(result.Content[0].Text)

	return false
}

//nolint:dupl // Avoid cleverness.
func (c *client) callCreateDirectory() bool {
	fmt.Println("Enter relative path (from the root) to the directory:")

	input, err := waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	params := mcp.ToolsCallParams{
		Name: "create_directory",
		Arguments: map[string]any{
			"path": input,
		},
	}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsCall,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return true
	case out = <-c.outChan:
	}

	if out.Error != nil {
		fmt.Printf("result error: %v\n", out.Error)
		return false
	}

	var result mcp.ToolResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("Directory is empty")
		return false
	}

	fmt.Println(result.Content[0].Text)

	return false
}

//nolint:dupl // Avoid cleverness.
func (c *client) callListDirectory() bool {
	fmt.Println("Enter relative path (from the root) to the directory:")

	input, err := waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	params := mcp.ToolsCallParams{
		Name: "list_directory",
		Arguments: map[string]any{
			"path": input,
		},
	}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsCall,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return true
	case out = <-c.outChan:
	}

	if out.Error != nil {
		fmt.Printf("result error: %v\n", out.Error)
		return false
	}

	var result mcp.ToolResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
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

//nolint:dupl // Avoid cleverness.
func (c *client) callDirectoryTree() bool {
	fmt.Println("Enter relative path (from the root) to the directory:")

	input, err := waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	params := mcp.ToolsCallParams{
		Name: "directory_tree",
		Arguments: map[string]any{
			"path": input,
		},
	}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsCall,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return true
	case out = <-c.outChan:
	}

	if out.Error != nil {
		fmt.Printf("result error: %v\n", out.Error)
		return false
	}

	var result mcp.ToolResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
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

//nolint:dupl // Avoid cleverness.
func (c *client) callMoveFile() bool {
	fmt.Println("Enter relative path (from the root) to the source file:")

	input, err := waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	path := input

	fmt.Println("Enter relative path (from the root) to the destination file:")

	input, err = waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}
	destination := input

	params := mcp.ToolsCallParams{
		Name: "move_file",
		Arguments: map[string]any{
			"source":      path,
			"destination": destination,
		},
	}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsCall,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return true
	case out = <-c.outChan:
	}

	if out.Error != nil {
		fmt.Printf("result error: %v\n", out.Error)
		return false
	}

	var result mcp.ToolResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("File is empty")
		return false
	}

	fmt.Println(result.Content[0].Text)

	return false
}

func (c *client) callSearchFiles() bool {
	fmt.Println("Enter pattern:")

	input, err := waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	pattern := input

	fmt.Println("Enter exclude patterns (comma-separated):")

	input, err = waitStdIOInput(c.ctx)
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

	params := mcp.ToolsCallParams{
		Name: "search_files",
		Arguments: map[string]any{
			"path":            pattern,
			"pattern":         pattern,
			"excludePatterns": excludePatterns,
		},
	}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsCall,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return true
	case out = <-c.outChan:
	}

	if out.Error != nil {
		fmt.Printf("result error: %v\n", out.Error)
		return false
	}

	var result mcp.ToolResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
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

//nolint:dupl // Avoid cleverness.
func (c *client) callGetFileInfo() bool {
	fmt.Println("Enter relative path (from the root) to the file:")

	input, err := waitStdIOInput(c.ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		fmt.Print(err)
		return false
	}

	params := mcp.ToolsCallParams{
		Name: "get_file_info",
		Arguments: map[string]any{
			"path": input,
		},
	}
	paramsBs, _ := json.Marshal(params)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodToolsCall,
		Params:  paramsBs,
	}

	var out mcp.JSONRPCMessage

	select {
	case <-c.ctx.Done():
		return true
	case out = <-c.outChan:
	}

	if out.Error != nil {
		fmt.Printf("result error: %v\n", out.Error)
		return false
	}

	var result mcp.ToolResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
		return false
	}

	if len(result.Content) == 0 {
		fmt.Println("File is empty")
		return false
	}

	fmt.Println(result.Content[0].Text)

	return false
}
