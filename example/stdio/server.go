package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

type server struct{}

func (s server) Info() mcp.Info {
	return mcp.Info{
		Name:    "test-server",
		Version: "1.0",
	}
}

func (s server) RequireSamplingClient() bool {
	return false
}

func (s server) ListPrompts(_ context.Context, params mcp.PromptsListParams) (mcp.PromptList, error) {
	crs := params.Cursor
	if crs == "" {
		crs = "0"
	}
	crsInt, err := strconv.Atoi(crs)
	if err != nil {
		return mcp.PromptList{}, fmt.Errorf("invalid cursor: %w", err)
	}

	nc := crsInt + 1
	ncStr := strconv.Itoa(nc)
	if nc >= len(s.prompts()) {
		ncStr = ""
	}

	return mcp.PromptList{
		Prompts:    s.prompts()[crsInt],
		NextCursor: ncStr,
	}, nil
}

func (s server) GetPrompt(_ context.Context, params mcp.PromptsGetParams) (mcp.PromptResult, error) {
	found := false

	for _, ps := range s.prompts() {
		idx := slices.IndexFunc(ps, func(p mcp.Prompt) bool {
			return p.Name == params.Name
		})
		if idx > -1 {
			found = true
			break
		}
	}

	if !found {
		return mcp.PromptResult{}, fmt.Errorf("prompt not found")
	}

	return s.promptResults()[params.Name], nil
}

func (s server) CompletesPrompt(_ context.Context, _ mcp.CompletionCompleteParams) (mcp.CompletionResult, error) {
	return mcp.CompletionResult{}, nil
}

func (s server) prompts() [][]mcp.Prompt {
	return [][]mcp.Prompt{
		{
			{
				Name:        "test-prompt-1",
				Description: "Test Prompt 1",
				Arguments: []mcp.PromptArgument{
					{Name: "arg1", Description: "Argument 1", Required: true},
				},
			},
		},
		{
			{
				Name:        "test-prompt-2",
				Description: "Test Prompt 2",
				Arguments: []mcp.PromptArgument{
					{Name: "arg2", Description: "Argument 2", Required: false},
				},
			},
		},
		{
			{
				Name:        "test-prompt-3",
				Description: "Test Prompt 3",
				Arguments: []mcp.PromptArgument{
					{Name: "arg3", Description: "Argument 3", Required: true},
				},
			},
		},
	}
}

func (s server) promptResults() map[string]mcp.PromptResult {
	return map[string]mcp.PromptResult{
		"test-prompt-1": {
			Description: "Test Prompt 1",
			Messages: []mcp.PromptMessage{
				{
					Role: mcp.PromptRoleUser,
					Content: mcp.Content{
						Type: mcp.ContentTypeText,
						Text: "Hello",
					},
				},
			},
		},
		"test-prompt-2": {
			Description: "Test Prompt 2",
			Messages: []mcp.PromptMessage{
				{
					Role: mcp.PromptRoleAssistant,
					Content: mcp.Content{
						Type: mcp.ContentTypeText,
						Text: "Hello",
					},
				},
				{
					Role: mcp.PromptRoleUser,
					Content: mcp.Content{
						Type: mcp.ContentTypeText,
						Text: "World",
					},
				},
			},
		},
		"test-prompt-3": {
			Description: "Test Prompt 3",
			Messages: []mcp.PromptMessage{
				{
					Role: mcp.PromptRoleAssistant,
					Content: mcp.Content{
						Type: mcp.ContentTypeText,
						Text: "Hello",
					},
				},
				{
					Role: mcp.PromptRoleUser,
					Content: mcp.Content{
						Type: mcp.ContentTypeText,
						Text: "MCP",
					},
				},
			},
		},
	}
}
