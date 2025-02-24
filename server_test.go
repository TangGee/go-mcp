package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/MegaGrindStone/go-mcp"
)

type mockPromptServer struct {
	listParams      mcp.ListPromptsParams
	getParams       mcp.GetPromptParams
	completesParams mcp.CompletesCompletionParams
}

type mockPromptListUpdater struct {
	ch   chan struct{}
	done chan struct{}
}

type mockResourceServer struct {
	delayList               bool
	listParams              mcp.ListResourcesParams
	readParams              mcp.ReadResourceParams
	listTemplatesParams     mcp.ListResourceTemplatesParams
	completesTemplateParams mcp.CompletesCompletionParams
}

type mockResourceListUpdater struct {
	ch   chan struct{}
	done chan struct{}
}

type mockResourceSubscriptionHandler struct {
	subscribeParams   mcp.SubscribeResourceParams
	unsubscribeParams mcp.UnsubscribeResourceParams
	ch                chan string
	done              chan struct{}
}

type mockToolServer struct {
	listParams mcp.ListToolsParams
	callParams mcp.CallToolParams

	requestRootsList bool
	requestSampling  bool
}

type mockToolListUpdater struct {
	ch   chan struct{}
	done chan struct{}
}

type mockLogHandler struct {
	lock  sync.Mutex
	level mcp.LogLevel

	params chan mcp.LogParams
	done   chan struct{}
}

type mockRootsListWatcher struct {
	lock        sync.Mutex
	updateCount int
}

func (m *mockPromptServer) ListPrompts(
	_ context.Context,
	params mcp.ListPromptsParams,
	progressReporter mcp.ProgressReporter,
	_ mcp.RequestClientFunc,
) (mcp.ListPromptResult, error) {
	for i := 0; i < 10; i++ {
		progressReporter(mcp.ProgressParams{
			ProgressToken: params.Meta.ProgressToken,
			Progress:      float64(i) / 10,
			Total:         10,
		})
	}
	m.listParams = params
	return mcp.ListPromptResult{}, nil
}

func (m *mockPromptServer) GetPrompt(
	_ context.Context,
	params mcp.GetPromptParams,
	_ mcp.ProgressReporter,
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

func (m mockPromptListUpdater) PromptListUpdates() iter.Seq[struct{}] {
	return func(yield func(struct{}) bool) {
		for {
			select {
			case <-m.done:
				return
			case <-m.ch:
				if !yield(struct{}{}) {
					return
				}
			}
		}
	}
}

func (m *mockResourceServer) ListResources(
	ctx context.Context,
	params mcp.ListResourcesParams,
	_ mcp.ProgressReporter,
	_ mcp.RequestClientFunc,
) (mcp.ListResourcesResult, error) {
	if m.delayList {
		select {
		case <-ctx.Done():
			return mcp.ListResourcesResult{}, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	m.listParams = params
	return mcp.ListResourcesResult{}, nil
}

func (m *mockResourceServer) ReadResource(
	_ context.Context,
	params mcp.ReadResourceParams,
	_ mcp.ProgressReporter,
	_ mcp.RequestClientFunc,
) (mcp.ReadResourceResult, error) {
	m.readParams = params
	return mcp.ReadResourceResult{}, nil
}

func (m *mockResourceServer) ListResourceTemplates(
	_ context.Context,
	params mcp.ListResourceTemplatesParams,
	_ mcp.ProgressReporter,
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

func (m mockResourceListUpdater) ResourceListUpdates() iter.Seq[struct{}] {
	return func(yield func(struct{}) bool) {
		for {
			select {
			case <-m.done:
				return
			case <-m.ch:
			}
			if !yield(struct{}{}) {
				return
			}
		}
	}
}

func (m *mockResourceSubscriptionHandler) SubscribeResource(params mcp.SubscribeResourceParams) {
	m.subscribeParams = params
}

func (m *mockResourceSubscriptionHandler) UnsubscribeResource(params mcp.UnsubscribeResourceParams) {
	m.unsubscribeParams = params
}

func (m *mockResourceSubscriptionHandler) SubscribedResourceUpdates() iter.Seq[string] {
	return func(yield func(string) bool) {
		for {
			select {
			case <-m.done:
				return
			case uri := <-m.ch:
				if !yield(uri) {
					return
				}
			}
		}
	}
}

func (m *mockToolServer) ListTools(
	_ context.Context,
	params mcp.ListToolsParams,
	_ mcp.ProgressReporter,
	clientFunc mcp.RequestClientFunc,
) (mcp.ListToolsResult, error) {
	m.listParams = params
	if m.requestRootsList {
		_, err := clientFunc(mcp.JSONRPCMessage{
			JSONRPC: mcp.JSONRPCVersion,
			Method:  mcp.MethodRootsList,
		})
		if err != nil {
			return mcp.ListToolsResult{}, err
		}
	}
	return mcp.ListToolsResult{}, nil
}

func (m *mockToolServer) CallTool(
	_ context.Context,
	params mcp.CallToolParams,
	_ mcp.ProgressReporter,
	clientFunc mcp.RequestClientFunc,
) (mcp.CallToolResult, error) {
	if m.requestSampling {
		samplingParams := mcp.SamplingParams{}
		samplingParamsBs, err := json.Marshal(samplingParams)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to marshal sampling params: %w", err)
		}
		_, err = clientFunc(mcp.JSONRPCMessage{
			JSONRPC: mcp.JSONRPCVersion,
			Method:  mcp.MethodSamplingCreateMessage,
			Params:  samplingParamsBs,
		})
		if err != nil {
			return mcp.CallToolResult{}, err
		}
	}
	m.callParams = params
	return mcp.CallToolResult{}, nil
}

func (m mockToolListUpdater) ToolListUpdates() iter.Seq[struct{}] {
	return func(yield func(struct{}) bool) {
		for {
			select {
			case <-m.done:
				return
			case <-m.ch:
				if !yield(struct{}{}) {
					return
				}
			}
		}
	}
}

func (m *mockLogHandler) LogStreams() iter.Seq[mcp.LogParams] {
	return func(yield func(mcp.LogParams) bool) {
		for {
			select {
			case <-m.done:
				return
			case params := <-m.params:
				if !yield(params) {
					return
				}
			}
		}
	}
}

func (m *mockLogHandler) SetLogLevel(level mcp.LogLevel) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.level = level
}

func (m *mockRootsListWatcher) OnRootsListChanged() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.updateCount++
}
