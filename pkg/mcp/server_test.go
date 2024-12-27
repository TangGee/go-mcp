package mcp_test

import (
	"context"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
	"github.com/qri-io/jsonschema"
)

type mockServer struct {
	requireRootsListClient bool
	requireSamplingClient  bool
}

type mockPromptServer struct{}

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
	context.Context,
	mcp.PromptsListParams,
	mcp.RequestClientFunc,
) (mcp.PromptList, error) {
	return mcp.PromptList{
		Prompts: []mcp.Prompt{
			{Name: "test-prompt"},
		},
	}, nil
}

func (m mockPromptServer) GetPrompt(
	context.Context,
	mcp.PromptsGetParams,
	mcp.RequestClientFunc,
) (mcp.PromptResult, error) {
	return mcp.PromptResult{
		Description: "Test Prompt",
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
	mcp.CompletionCompleteParams,
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
	mcp.ResourcesListParams,
	mcp.RequestClientFunc,
) (mcp.ResourceList, error) {
	return mcp.ResourceList{
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
	mcp.ResourcesReadParams,
	mcp.RequestClientFunc,
) (mcp.Resource, error) {
	return mcp.Resource{
		URI:         "test://resource",
		Name:        "Test Resource",
		Description: "A test resource",
		MimeType:    "text/plain",
		Text:        "This is the resource content",
	}, nil
}

func (m mockResourceServer) ListResourceTemplates(
	context.Context,
	mcp.ResourcesTemplatesListParams,
	mcp.RequestClientFunc,
) ([]mcp.ResourceTemplate, error) {
	return []mcp.ResourceTemplate{
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
	}, nil
}

func (m mockResourceServer) CompletesResourceTemplate(
	context.Context,
	mcp.CompletionCompleteParams,
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

func (m mockToolServer) ListTools(context.Context, mcp.ToolsListParams, mcp.RequestClientFunc) (mcp.ToolList, error) {
	return mcp.ToolList{
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

func (m mockToolServer) CallTool(context.Context, mcp.ToolsCallParams, mcp.RequestClientFunc) (mcp.ToolResult, error) {
	return mcp.ToolResult{
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
