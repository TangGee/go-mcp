package mcp_test

import (
	"context"
	"fmt"
	"slices"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
	"github.com/qri-io/jsonschema"
)

type mockServer struct {
	requireRootsListClient bool
	requireSamplingClient  bool
}

type mockPromptServer struct {
	prompts  []mcp.Prompt
	pageSize int
}

type mockPromptListUpdater struct{}

type mockResourceServer struct{}

type mockResourceListUpdater struct{}

type mockResourceSubscribedUpdater struct{}

type mockToolServer struct{}

type mockToolListUpdater struct{}

type mockLogHandler struct{}

type mockRootsListWatcher struct{}

func (m mockServer) Info() mcp.Info {
	return mcp.Info{Name: "test-server", Version: "1.0"}
}

func (m mockServer) RequireRootsListClient() bool {
	return m.requireRootsListClient
}

func (m mockServer) RequireSamplingClient() bool {
	return m.requireSamplingClient
}

func (m mockPromptServer) ListPrompts(
	_ context.Context,
	params mcp.ListPromptsParams,
	_ mcp.RequestClientFunc,
) (mcp.ListPromptResult, error) {
	startIndex, endIndex, nextCursor := getPageInfo(params.Cursor, m.pageSize, len(m.prompts))
	return mcp.ListPromptResult{
		Prompts:    m.prompts[startIndex:endIndex],
		NextCursor: nextCursor,
	}, nil
}

func (m mockPromptServer) GetPrompt(
	_ context.Context,
	params mcp.GetPromptParams,
	_ mcp.RequestClientFunc,
) (mcp.GetPromptResult, error) {
	idx := slices.IndexFunc(m.prompts, func(p mcp.Prompt) bool {
		return p.Name == params.Name
	})
	if idx == -1 {
		return mcp.GetPromptResult{}, fmt.Errorf("prompt not found")
	}
	return mcp.GetPromptResult{
		Description: m.prompts[idx].Description,
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.PromptRoleAssistant,
				Content: mcp.Content{
					Type: mcp.ContentTypeText,
					Text: "Test response message",
				},
			},
		},
	}, nil
}

func (m mockPromptServer) CompletesPrompt(
	context.Context,
	mcp.CompletesCompletionParams,
	mcp.RequestClientFunc,
) (mcp.CompletionResult, error) {
	return mcp.CompletionResult{
		Completion: struct {
			Values  []string `json:"values"`
			HasMore bool     `json:"hasMore"`
		}{
			Values:  []string{"test-value1", "test-value2"},
			HasMore: true,
		},
	}, nil
}

func (m mockPromptListUpdater) PromptListUpdates() <-chan struct{} {
	return nil
}

func (m mockResourceServer) ListResources(
	context.Context,
	mcp.ListResourcesParams,
	mcp.RequestClientFunc,
) (mcp.ListResourcesResult, error) {
	return mcp.ListResourcesResult{
		Resources: []mcp.Resource{
			{
				URI:         "test://resource1",
				Name:        "Test Resource 1",
				Description: "First test resource",
				MimeType:    "text/plain",
				Text:        "Resource 1 content",
			},
			{
				URI:         "test://resource2",
				Name:        "Test Resource 2",
				Description: "Second test resource",
				MimeType:    "application/json",
				Text:        "{\"key\": \"value\"}",
			},
		},
	}, nil
}

func (m mockResourceServer) ReadResource(
	context.Context,
	mcp.ReadResourceParams,
	mcp.RequestClientFunc,
) (mcp.ReadResourceResult, error) {
	return mcp.ReadResourceResult{
		Contents: []mcp.Resource{
			{
				URI:         "test://resource",
				Name:        "Test Resource",
				Description: "A test resource",
				MimeType:    "text/plain",
				Text:        "This is the resource content",
			},
		},
	}, nil
}

func (m mockResourceServer) ListResourceTemplates(
	context.Context,
	mcp.ListResourceTemplatesParams,
	mcp.RequestClientFunc,
) (mcp.ListResourceTemplatesResult, error) {
	return mcp.ListResourceTemplatesResult{
		Templates: []mcp.ResourceTemplate{
			{
				URITemplate: "test://resource/{name}",
				Name:        "Test Template 1",
				Description: "First test template",
				MimeType:    "text/plain",
			},
			{
				URITemplate: "test://resource/{id}",
				Name:        "Test Template 2",
				Description: "Second test template",
				MimeType:    "application/json",
			},
		},
	}, nil
}

func (m mockResourceServer) CompletesResourceTemplate(
	context.Context,
	mcp.CompletesCompletionParams,
	mcp.RequestClientFunc,
) (mcp.CompletionResult, error) {
	return mcp.CompletionResult{}, nil
}

func (m mockResourceServer) SubscribeResource(mcp.ResourcesSubscribeParams) {
}

func (m mockResourceServer) UnsubscribeResource(mcp.ResourcesSubscribeParams) {
}

func (m mockResourceListUpdater) ResourceListUpdates() <-chan struct{} {
	return nil
}

func (m mockResourceSubscribedUpdater) ResourceSubscribedUpdates() <-chan string {
	return nil
}

func (m mockToolServer) ListTools(context.Context, mcp.ListToolsParams, mcp.RequestClientFunc) (mcp.ListToolsResult, error) {
	return mcp.ListToolsResult{
		Tools: []mcp.Tool{
			{
				Name:        "test-tool",
				Description: "Test Tool",
				InputSchema: jsonschema.Must(`{
            "type": "object",
            "properties": {
              "param1": { "type": "string" }
            }
          }`),
			},
		},
	}, nil
}

func (m mockToolServer) CallTool(context.Context, mcp.CallToolParams, mcp.RequestClientFunc) (mcp.CallToolResult, error) {
	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: "Tool execution result",
			},
			{
				Type:     mcp.ContentTypeImage,
				Data:     "base64data",
				MimeType: "image/png",
			},
		},
		IsError: false,
	}, nil
}

func (m mockToolListUpdater) ToolListUpdates() <-chan struct{} {
	return nil
}

func (m mockLogHandler) LogStreams() <-chan mcp.LogParams {
	return nil
}

func (m mockLogHandler) SetLogLevel(mcp.LogLevel) {
}

func (m mockRootsListWatcher) OnRootsListChanged() {
}
