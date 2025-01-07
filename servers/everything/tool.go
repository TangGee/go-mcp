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
)

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
func (s *Server) ListTools(
	context.Context,
	mcp.ListToolsParams,
	mcp.ProgressReporter,
	mcp.RequestClientFunc,
) (mcp.ListToolsResult, error) {
	s.log("ListTools", mcp.LogLevelDebug)

	return mcp.ListToolsResult{
		Tools: toolList,
	}, nil
}

// CallTool implements mcp.ToolServer interface.
func (s *Server) CallTool(
	_ context.Context,
	params mcp.CallToolParams,
	progressReporter mcp.ProgressReporter,
	requestClient mcp.RequestClientFunc,
) (mcp.CallToolResult, error) {
	s.log(fmt.Sprintf("CallTool: %s", params.Name), mcp.LogLevelDebug)

	switch params.Name {
	case "echo":
		return s.callEcho(params)
	case "add":
		return s.callAdd(params)
	case "longRunningOperation":
		return s.callLongRunningOperation(params, progressReporter)
	case "printEnv":
		return s.callPrintEnv(params)
	case "sampleLLM":
		return s.callSampleLLM(params, requestClient)
	case "getTinyImage":
		return s.callGetTinyImage(params)
	default:
		return mcp.CallToolResult{}, fmt.Errorf("tool not found: %s", params.Name)
	}
}

func (s *Server) callEcho(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var eArgs EchoArgs
	if err := json.Unmarshal(params.Arguments, &eArgs); err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: eArgs.Message,
			},
		},
		IsError: false,
	}, nil
}

func (s *Server) callAdd(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var aArgs AddArgs
	if err := json.Unmarshal(params.Arguments, &aArgs); err != nil {
		return mcp.CallToolResult{}, err
	}

	result := aArgs.A + aArgs.B

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("The sum of %f and %f is %f", aArgs.A, aArgs.B, result),
			},
		},
		IsError: false,
	}, nil
}

func (s *Server) callLongRunningOperation(
	params mcp.CallToolParams,
	progressReporter mcp.ProgressReporter,
) (mcp.CallToolResult, error) {
	var lroArgs LongRunningOperationArgs
	if err := json.Unmarshal(params.Arguments, &lroArgs); err != nil {
		return mcp.CallToolResult{}, err
	}

	stepDuration := lroArgs.Duration / lroArgs.Steps

	for i := 0; i < int(lroArgs.Steps); i++ {
		time.Sleep(time.Duration(stepDuration) * time.Second)

		if params.Meta.ProgressToken == "" {
			continue
		}

		progressReporter(mcp.ProgressParams{
			ProgressToken: params.Meta.ProgressToken,
			Progress:      float64(i + 1),
			Total:         lroArgs.Steps,
		})
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("Long running operation completed. Duration: %f seconds, Steps: %f",
					lroArgs.Duration, lroArgs.Steps),
			},
		},
		IsError: false,
	}, nil
}

func (s *Server) callPrintEnv(_ mcp.CallToolParams) (mcp.CallToolResult, error) {
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
	params mcp.CallToolParams,
	requestClient mcp.RequestClientFunc,
) (mcp.CallToolResult, error) {
	var sllArgs SampleLLMArgs
	if err := json.Unmarshal(params.Arguments, &sllArgs); err != nil {
		return mcp.CallToolResult{}, err
	}

	samplingParams := mcp.SamplingParams{
		Messages: []mcp.SamplingMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.SamplingContent{
					Type: "text",
					Text: fmt.Sprintf("Resource sampleLLM context: %s", sllArgs.Prompt),
				},
			},
		},
		ModelPreferences: mcp.SamplingModelPreferences{
			CostPriority:         1,
			SpeedPriority:        2,
			IntelligencePriority: 3,
		},
		SystemPrompts: "You are a helpful assistant.",
		MaxTokens:     int(sllArgs.MaxTokens),
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

func (s *Server) callGetTinyImage(_ mcp.CallToolParams) (mcp.CallToolResult, error) {
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
