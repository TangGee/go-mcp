package mcp_test

import (
	"context"
	"sync"

	"github.com/MegaGrindStone/go-mcp"
)

type mockPromptListWatcher struct {
	lock        sync.Mutex
	updateCount int
}

type mockResourceListWatcher struct{}

type mockResourceSubscribedWatcher struct{}

type mockToolListWatcher struct{}

type mockRootsListHandler struct{}

type mockRootsListUpdater struct {
	ch chan struct{}
}

type mockSamplingHandler struct{}

type mockProgressListener struct {
	params []mcp.ProgressParams
}

type mockLogReceiver struct {
	lock   sync.Mutex
	params []mcp.LogParams
}

func (m *mockPromptListWatcher) OnPromptListChanged() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.updateCount++
}

func (m mockResourceListWatcher) OnResourceListChanged() {
}

func (m mockResourceSubscribedWatcher) OnResourceSubscribedChanged(string) {
}

func (m mockToolListWatcher) OnToolListChanged() {
}

func (m mockRootsListHandler) RootsList(context.Context) (mcp.RootList, error) {
	return mcp.RootList{
		Roots: []mcp.Root{
			{URI: "test://root", Name: "Test Root"},
		},
	}, nil
}

func (m mockRootsListUpdater) RootsListUpdates() <-chan struct{} {
	if m.ch == nil {
		m.ch = make(chan struct{})
	}
	return m.ch
}

func (m mockSamplingHandler) CreateSampleMessage(context.Context, mcp.SamplingParams) (mcp.SamplingResult, error) {
	return mcp.SamplingResult{
		Role: mcp.PromptRoleAssistant,
		Content: mcp.SamplingContent{
			Type: "text",
			Text: "Test response",
		},
		Model:      "test-model",
		StopReason: "completed",
	}, nil
}

func (m *mockLogReceiver) OnLog(params mcp.LogParams) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.params = append(m.params, params)
}

func (m *mockProgressListener) OnProgress(params mcp.ProgressParams) {
	m.params = append(m.params, params)
}
