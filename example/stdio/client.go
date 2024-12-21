package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

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
		inChan:  make(chan mcp.JSONRPCMessage),
		outChan: make(chan mcp.JSONRPCMessage),
	}
}

func (c *client) Info() mcp.Info {
	return mcp.Info{
		Name:    "test-client",
		Version: "1.0",
	}
}

func (c *client) RequirePromptServer() bool {
	return true
}

func (c *client) RequireResourceServer() bool {
	return false
}

func (c *client) RequireToolServer() bool {
	return false
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

func (c *client) prompts() bool {
	cursor := ""
	for {
		params := mcp.PromptsListParams{
			Cursor: cursor,
		}
		paramsBs, _ := json.Marshal(params)
		c.inChan <- mcp.JSONRPCMessage{
			JSONRPC: mcp.JSONRPCVersion,
			ID:      mcp.MustString(uuid.New().String()),
			Method:  mcp.MethodPromptsList,
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
			break
		}
		var result mcp.PromptList
		if err := json.Unmarshal(out.Result, &result); err != nil {
			fmt.Printf("failed to unmarshal result: %v\n", err)
			break
		}

		fmt.Println("List prompts:")
		fmt.Println()
		for _, prompt := range result.Prompts {
			fmt.Printf("%s: %s\n", prompt.Name, prompt.Description)
		}
		fmt.Println()

		askStr := "Type 'n' for next page, "
		if result.NextCursor == "" {
			askStr = "No more pages, type 'n' for start over, "
		}
		askStr += "or type 'm' to go back to main menu, "

		fmt.Printf("%sor type prompt name to choose prompt: ", askStr)

		input, err := waitStdIOInput(c.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return true
			}
			fmt.Print(err)
			break
		}

		if input == "n" {
			cursor = result.NextCursor
			continue
		}
		if input == "m" {
			break
		}

		resultIdx := slices.IndexFunc(result.Prompts, func(p mcp.Prompt) bool {
			return p.Name == input
		})
		if resultIdx == -1 {
			fmt.Printf("prompt not found: %s\n", input)
			continue
		}

		return c.getPrompt(input)
	}

	return false
}

func (c *client) getPrompt(name string) bool {
	getParams := mcp.PromptsGetParams{
		Name: name,
	}
	getParamsBs, _ := json.Marshal(getParams)
	c.inChan <- mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodPromptsGet,
		Params:  getParamsBs,
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

	var result mcp.Prompt
	if err := json.Unmarshal(out.Result, &result); err != nil {
		fmt.Printf("failed to unmarshal result: %v\n", err)
		return false
	}

	fmt.Println()
	fmt.Println("Prompt:")
	fmt.Printf("Name: %s\n", result.Name)
	fmt.Printf("Description: %s\n", result.Description)
	fmt.Println()

	return false
}
