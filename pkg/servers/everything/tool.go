package everything

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
	"github.com/qri-io/jsonschema"
)

var echoSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "message": { "type": "string" }
  }
}`)

var addSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "a": { "type": "number" },
    "b": { "type": "number" }
  }
}`)

var longRunningOperationSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "duration": { "type": "number", "default": 10 },
    "steps": { "type": "number", "default": 5 }
  }
}`)

var sampleLLMSchema = jsonschema.Must(`{
  "type": "object",
  "properties": {
    "prompt": { "type": "string" },
    "maxTokens": { "type": "number", "default": 100 }
  }
}`)

var toolList = []mcp.Tool{
	{
		Name:        "echo",
		Description: "Echoes back the input",
		InputSchema: echoSchema,
	},
	{
		Name:        "add",
		Description: "Adds two numbers",
		InputSchema: addSchema,
	},
	{
		Name:        "longRunningOperation",
		Description: "Demonstrates a long running operation with progress updates",
		InputSchema: longRunningOperationSchema,
	},
	{
		Name:        "printEnv",
		Description: "Prints all environment variables, helpful for debugging MCP server configuration",
	},
	{
		Name:        "sampleLLM",
		Description: "Samples from an LLM using MCP's sampling feature",
		InputSchema: sampleLLMSchema,
	},
	{
		Name:        "getTinyImage",
		Description: "Returns the MCP_TINY_IMAGE",
	},
}

// ListTools implements mcp.ToolServer interface.
func (s *SSEServer) ListTools(_ context.Context, _ mcp.ToolsListParams) (mcp.ToolList, error) {
	s.log("ListTools", mcp.LogLevelDebug)

	return mcp.ToolList{
		Tools: toolList,
	}, nil
}

// CallTool implements mcp.ToolServer interface.
func (s *SSEServer) CallTool(ctx context.Context, params mcp.ToolsCallParams) (mcp.ToolResult, error) {
	s.log(fmt.Sprintf("CallTool: %s", params.Name), mcp.LogLevelDebug)

	switch params.Name {
	case "echo":
		return s.callEcho(ctx, params)
	case "add":
		return s.callAdd(ctx, params)
	case "longRunningOperation":
		return s.callLongRunningOperation(ctx, params)
	case "printEnv":
		return s.callPrintEnv(ctx, params)
	case "sampleLLM":
		return s.callSampleLLM(ctx, params)
	case "getTinyImage":
		return s.callGetTinyImage(ctx, params)
	default:
		return mcp.ToolResult{}, fmt.Errorf("tool not found: %s", params.Name)
	}
}

func (s *SSEServer) callEcho(ctx context.Context, params mcp.ToolsCallParams) (mcp.ToolResult, error) {
	vs := echoSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	message, _ := params.Arguments["message"].(string)

	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: message,
			},
		},
		IsError: false,
	}, nil
}

func (s *SSEServer) callAdd(ctx context.Context, params mcp.ToolsCallParams) (mcp.ToolResult, error) {
	vs := addSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	a, _ := params.Arguments["a"].(float64)
	b, _ := params.Arguments["b"].(float64)

	result := a + b

	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("The sum of %f and %f is %f", a, b, result),
			},
		},
		IsError: false,
	}, nil
}

func (s *SSEServer) callLongRunningOperation(ctx context.Context, params mcp.ToolsCallParams) (mcp.ToolResult, error) {
	vs := longRunningOperationSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	duration, _ := params.Arguments["duration"].(float64)
	steps, _ := params.Arguments["steps"].(float64)
	stepDuration := duration / steps

	for i := 0; i < int(steps); i++ {
		time.Sleep(time.Duration(stepDuration) * time.Second)

		if params.Meta.ProgressToken == "" {
			continue
		}

		select {
		case s.progressChan <- mcp.ProgressParams{
			ProgressToken: params.Meta.ProgressToken,
			Progress:      float64(i + 1),
			Total:         steps,
		}:
		case <-ctx.Done():
			return mcp.ToolResult{}, ctx.Err()
		case <-s.doneChan:
			return mcp.ToolResult{}, fmt.Errorf("server closed")
		}
	}

	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("Long running operation completed. Duration: %f seconds, Steps: %f", duration, steps),
			},
		},
		IsError: false,
	}, nil
}

func (s *SSEServer) callPrintEnv(_ context.Context, _ mcp.ToolsCallParams) (mcp.ToolResult, error) {
	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("Environment variables:\n%s", strings.Join(os.Environ(), "\n")),
			},
		},
		IsError: false,
	}, nil
}

func (s *SSEServer) callSampleLLM(ctx context.Context, params mcp.ToolsCallParams) (mcp.ToolResult, error) {
	vs := sampleLLMSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.ToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	prompt, _ := params.Arguments["prompt"].(string)
	maxTokens, _ := params.Arguments["maxTokens"].(float64)

	sample, err := s.MCPServer.RequestSampling(ctx, mcp.SamplingParams{
		Messages: []mcp.SamplingMessage{
			{
				Role: mcp.PromptRoleUser,
				Content: mcp.SamplingContent{
					Type: "text",
					Text: fmt.Sprintf("Resource sampleLLM context: %s", prompt),
				},
			},
		},
		ModelPreferences: mcp.SamplingModelPreferences{
			CostPriority:         1,
			SpeedPriority:        2,
			IntelligencePriority: 3,
		},
		SystemPrompts: "You are a helpful assistant.",
		MaxTokens:     int(maxTokens),
	})
	if err != nil {
		return mcp.ToolResult{}, fmt.Errorf("failed to request sampling: %w", err)
	}

	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: sample.Content.Text,
			},
		},
		IsError: false,
	}, nil
}

func (s *SSEServer) callGetTinyImage(_ context.Context, _ mcp.ToolsCallParams) (mcp.ToolResult, error) {
	return mcp.ToolResult{
		Content: []mcp.Content{
			{
				Type:     mcp.ContentTypeImage,
				Data:     mcpTinyImage,
				MimeType: "image/png",
			},
		},
		IsError: false,
	}, nil
}
