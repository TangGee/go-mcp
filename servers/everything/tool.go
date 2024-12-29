package everything

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MegaGrindStone/go-mcp"
	"github.com/google/uuid"
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
func (s *Server) ListTools(context.Context, mcp.ListToolsParams, mcp.RequestClientFunc) (mcp.ListToolsResult, error) {
	s.log("ListTools", mcp.LogLevelDebug)

	return mcp.ListToolsResult{
		Tools: toolList,
	}, nil
}

// CallTool implements mcp.ToolServer interface.
func (s *Server) CallTool(
	ctx context.Context,
	params mcp.CallToolParams,
	requestClient mcp.RequestClientFunc,
) (mcp.CallToolResult, error) {
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
		return s.callSampleLLM(ctx, params, requestClient)
	case "getTinyImage":
		return s.callGetTinyImage(ctx, params)
	default:
		return mcp.CallToolResult{}, fmt.Errorf("tool not found: %s", params.Name)
	}
}

func (s *Server) callEcho(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	vs := echoSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.CallToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	message, _ := params.Arguments["message"].(string)

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: message,
			},
		},
		IsError: false,
	}, nil
}

func (s *Server) callAdd(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	vs := addSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.CallToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	a, _ := params.Arguments["a"].(float64)
	b, _ := params.Arguments["b"].(float64)

	result := a + b

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("The sum of %f and %f is %f", a, b, result),
			},
		},
		IsError: false,
	}, nil
}

func (s *Server) callLongRunningOperation(ctx context.Context, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	vs := longRunningOperationSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.CallToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
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
			return mcp.CallToolResult{}, ctx.Err()
		case <-s.doneChan:
			return mcp.CallToolResult{}, fmt.Errorf("server closed")
		}
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("Long running operation completed. Duration: %f seconds, Steps: %f", duration, steps),
			},
		},
		IsError: false,
	}, nil
}

func (s *Server) callPrintEnv(_ context.Context, _ mcp.CallToolParams) (mcp.CallToolResult, error) {
	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("Environment variables:\n%s", strings.Join(os.Environ(), "\n")),
			},
		},
		IsError: false,
	}, nil
}

func (s *Server) callSampleLLM(
	ctx context.Context,
	params mcp.CallToolParams,
	requestClient mcp.RequestClientFunc,
) (mcp.CallToolResult, error) {
	vs := sampleLLMSchema.Validate(ctx, params.Arguments)
	errs := *vs.Errs
	if len(errs) > 0 {
		var errStr []string
		for _, err := range errs {
			errStr = append(errStr, err.Message)
		}
		return mcp.CallToolResult{}, fmt.Errorf("params validation failed: %s", strings.Join(errStr, ", "))
	}

	prompt, _ := params.Arguments["prompt"].(string)
	maxTokens, _ := params.Arguments["maxTokens"].(float64)

	samplingParams := mcp.SamplingParams{
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
	}

	samplingParamsBs, err := json.Marshal(samplingParams)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to marshal sampling params: %w", err)
	}

	resMsg, err := requestClient(mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      mcp.MustString(uuid.New().String()),
		Method:  mcp.MethodSamplingCreateMessage,
		Params:  samplingParamsBs,
	})
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to request sampling: %w", err)
	}

	var samplingResult mcp.SamplingResult
	if err := json.Unmarshal(resMsg.Result, &samplingResult); err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to unmarshal sampling result: %w", err)
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: samplingResult.Content.Text,
			},
		},
		IsError: false,
	}, nil
}

func (s *Server) callGetTinyImage(_ context.Context, _ mcp.CallToolParams) (mcp.CallToolResult, error) {
	return mcp.CallToolResult{
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
