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

type mockResourceListWatcher struct {
	lock        sync.Mutex
	updateCount int
}

type mockResourceSubscribedWatcher struct {
	lock        sync.Mutex
	updateCount int
}

type mockToolListWatcher struct{}

type mockRootsListHandler struct{}

type mockRootsListUpdater struct {
	ch chan struct{}
}

type mockSamplingHandler struct{}

type mockProgressListener struct {
	lock        sync.Mutex
	updateCount int
}

type mockLogReceiver struct {
	lock        sync.Mutex
	updateCount int
}

func (m *mockPromptListWatcher) OnPromptListChanged() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.updateCount++
}

func (m *mockResourceListWatcher) OnResourceListChanged() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.updateCount++
}

func (m *mockResourceSubscribedWatcher) OnResourceSubscribedChanged(string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.updateCount++
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

func (m *mockLogReceiver) OnLog(mcp.LogParams) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.updateCount++
}

func (m *mockProgressListener) OnProgress(mcp.ProgressParams) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.updateCount++
}
