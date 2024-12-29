package mcp_test

import (
	"context"

	"github.com/MegaGrindStone/go-mcp"
)

type mockServer struct {
	requireRootsListClient bool
	requireSamplingClient  bool
}

type mockPromptServer struct {
	listParams      mcp.ListPromptsParams
	getParams       mcp.GetPromptParams
	completesParams mcp.CompletesCompletionParams
}

type mockPromptListUpdater struct{}

type mockResourceServer struct {
	listParams              mcp.ListResourcesParams
	readParams              mcp.ReadResourceParams
	listTemplatesParams     mcp.ListResourceTemplatesParams
	completesTemplateParams mcp.CompletesCompletionParams
	subscribeParams         mcp.SubscribeResourceParams
	unsubscribeParams       mcp.UnsubscribeResourceParams
}

type mockResourceListUpdater struct{}

type mockResourceSubscribedUpdater struct{}

type mockToolServer struct {
	listParams mcp.ListToolsParams
	callParams mcp.CallToolParams
}

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

func (m *mockPromptServer) ListPrompts(
	_ context.Context,
	params mcp.ListPromptsParams,
	_ mcp.RequestClientFunc,
) (mcp.ListPromptResult, error) {
	m.listParams = params
	return mcp.ListPromptResult{}, nil
}

func (m *mockPromptServer) GetPrompt(
	_ context.Context,
	params mcp.GetPromptParams,
	_ mcp.RequestClientFunc,
) (mcp.GetPromptResult, error) {
	m.getParams = params
	return mcp.GetPromptResult{}, nil
}

func (m *mockPromptServer) CompletesPrompt(
	_ context.Context,
	params mcp.CompletesCompletionParams,
	_ mcp.RequestClientFunc,
) (mcp.CompletionResult, error) {
	m.completesParams = params
	return mcp.CompletionResult{}, nil
}

func (m mockPromptListUpdater) PromptListUpdates() <-chan struct{} {
	return nil
}

func (m *mockResourceServer) ListResources(
	_ context.Context,
	params mcp.ListResourcesParams,
	_ mcp.RequestClientFunc,
) (mcp.ListResourcesResult, error) {
	m.listParams = params
	return mcp.ListResourcesResult{}, nil
}

func (m *mockResourceServer) ReadResource(
	_ context.Context,
	params mcp.ReadResourceParams,
	_ mcp.RequestClientFunc,
) (mcp.ReadResourceResult, error) {
	m.readParams = params
	return mcp.ReadResourceResult{}, nil
}

func (m *mockResourceServer) ListResourceTemplates(
	_ context.Context,
	params mcp.ListResourceTemplatesParams,
	_ mcp.RequestClientFunc,
) (mcp.ListResourceTemplatesResult, error) {
	m.listTemplatesParams = params
	return mcp.ListResourceTemplatesResult{}, nil
}

func (m *mockResourceServer) CompletesResourceTemplate(
	_ context.Context,
	params mcp.CompletesCompletionParams,
	_ mcp.RequestClientFunc,
) (mcp.CompletionResult, error) {
	m.completesTemplateParams = params
	return mcp.CompletionResult{}, nil
}

func (m *mockResourceServer) SubscribeResource(params mcp.SubscribeResourceParams) {
	m.subscribeParams = params
}

func (m *mockResourceServer) UnsubscribeResource(params mcp.UnsubscribeResourceParams) {
	m.unsubscribeParams = params
}

func (m mockResourceListUpdater) ResourceListUpdates() <-chan struct{} {
	return nil
}

func (m mockResourceSubscribedUpdater) ResourceSubscribedUpdates() <-chan string {
	return nil
}

func (m *mockToolServer) ListTools(
	_ context.Context,
	params mcp.ListToolsParams,
	_ mcp.RequestClientFunc,
) (mcp.ListToolsResult, error) {
	m.listParams = params
	return mcp.ListToolsResult{}, nil
}

func (m *mockToolServer) CallTool(
	_ context.Context,
	params mcp.CallToolParams,
	_ mcp.RequestClientFunc,
) (mcp.CallToolResult, error) {
	m.callParams = params
	return mcp.CallToolResult{}, nil
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
