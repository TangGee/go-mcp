package mcp_test

import (
	"context"
	"iter"
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

type mockRootsListHandler struct {
	called bool
}

type mockRootsListUpdater struct {
	ch chan struct{}
}

type mockSamplingHandler struct {
	called bool
}

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

func (m *mockRootsListHandler) RootsList(context.Context) (mcp.RootList, error) {
	m.called = true
	return mcp.RootList{
		Roots: []mcp.Root{
			{URI: "test://root", Name: "Test Root"},
		},
	}, nil
}

func (m mockRootsListUpdater) RootsListUpdates() iter.Seq[struct{}] {
	return func(yield func(struct{}) bool) {
		for range m.ch {
			if !yield(struct{}{}) {
				return
			}
		}
	}
}

func (m *mockSamplingHandler) CreateSampleMessage(context.Context, mcp.SamplingParams) (mcp.SamplingResult, error) {
	m.called = true
	return mcp.SamplingResult{
		Role: mcp.RoleAssistant,
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
