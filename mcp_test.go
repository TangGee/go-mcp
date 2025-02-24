package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/MegaGrindStone/go-mcp"
)

type testSuite struct {
	cfg testSuiteConfig

	clientCancel    context.CancelFunc
	serverTransport mcp.ServerTransport
	clientTransport mcp.ClientTransport

	httpServer  *httptest.Server
	srvIOReader *io.PipeReader
	srvIOWriter *io.PipeWriter
	cliIOReader *io.PipeReader
	cliIOWriter *io.PipeWriter

	mcpServer        mcp.Server
	mcpClient        *mcp.Client
	clientConnectErr error
}

type testSuiteConfig struct {
	transportName string
	serverOptions []mcp.ServerOption
	clientOptions []mcp.ClientOption
}

//nolint:gocognit
func TestInitialize(t *testing.T) {
	type testCase struct {
		name          string
		serverOptions []func() mcp.ServerOption
		clientOptions []func() mcp.ClientOption
		wantErr       bool
	}

	// Because these updaters needs to be created and closed on each test run,
	// we need to declare them as global variables.
	var promptListUpdater *mockPromptListUpdater
	var resourceListUpdater *mockResourceListUpdater
	var resourceSubscriptionHandler *mockResourceSubscriptionHandler
	var toolListUpdater *mockToolListUpdater
	var logHandler *mockLogHandler

	testCases := []testCase{
		{
			name:          "success with no capabilities",
			serverOptions: []func() mcp.ServerOption{},
			clientOptions: []func() mcp.ClientOption{},
			wantErr:       false,
		},
		{
			name: "success with full capabilities",
			serverOptions: []func() mcp.ServerOption{
				mcp.WithRequireRootsListClient,
				mcp.WithRequireSamplingClient,
				func() mcp.ServerOption {
					return mcp.WithPromptServer(&mockPromptServer{})
				},
				func() mcp.ServerOption {
					promptListUpdater = &mockPromptListUpdater{
						ch:   make(chan struct{}),
						done: make(chan struct{}),
					}
					return mcp.WithPromptListUpdater(promptListUpdater)
				},
				func() mcp.ServerOption {
					return mcp.WithResourceServer(&mockResourceServer{})
				},
				func() mcp.ServerOption {
					resourceListUpdater = &mockResourceListUpdater{
						ch:   make(chan struct{}),
						done: make(chan struct{}),
					}
					return mcp.WithResourceListUpdater(resourceListUpdater)
				},
				func() mcp.ServerOption {
					resourceSubscriptionHandler = &mockResourceSubscriptionHandler{
						ch:   make(chan string),
						done: make(chan struct{}),
					}
					return mcp.WithResourceSubscriptionHandler(resourceSubscriptionHandler)
				},
				func() mcp.ServerOption {
					return mcp.WithToolServer(&mockToolServer{})
				},
				func() mcp.ServerOption {
					toolListUpdater = &mockToolListUpdater{
						ch:   make(chan struct{}),
						done: make(chan struct{}),
					}
					return mcp.WithToolListUpdater(toolListUpdater)
				},
				func() mcp.ServerOption {
					logHandler = &mockLogHandler{
						lock:   sync.Mutex{},
						level:  mcp.LogLevelDebug,
						params: make(chan mcp.LogParams, 10),
						done:   make(chan struct{}),
					}
					return mcp.WithLogHandler(logHandler)
				},
				func() mcp.ServerOption {
					return mcp.WithRootsListWatcher(&mockRootsListWatcher{})
				},
			},
			clientOptions: []func() mcp.ClientOption{
				func() mcp.ClientOption {
					return mcp.WithPromptListWatcher(&mockPromptListWatcher{})
				},
				func() mcp.ClientOption {
					return mcp.WithResourceListWatcher(&mockResourceListWatcher{})
				},
				func() mcp.ClientOption {
					return mcp.WithResourceSubscribedWatcher(&mockResourceSubscribedWatcher{})
				},
				func() mcp.ClientOption {
					return mcp.WithToolListWatcher(&mockToolListWatcher{})
				},
				func() mcp.ClientOption {
					return mcp.WithRootsListHandler(&mockRootsListHandler{})
				},
				func() mcp.ClientOption {
					return mcp.WithRootsListUpdater(&mockRootsListUpdater{})
				},
				func() mcp.ClientOption {
					return mcp.WithSamplingHandler(&mockSamplingHandler{})
				},
				func() mcp.ClientOption {
					return mcp.WithLogReceiver(&mockLogReceiver{})
				},
			},
			wantErr: false,
		},
		{
			name: "fail insufficient client capabilities",
			serverOptions: []func() mcp.ServerOption{
				mcp.WithRequireSamplingClient,
				func() mcp.ServerOption {
					return mcp.WithPromptServer(&mockPromptServer{})
				},
			},
			clientOptions: []func() mcp.ClientOption{},
			wantErr:       true,
		},
	}

	for _, transportName := range []string{"SSE", "StdIO"} {
		for _, tc := range testCases {
			cfg := testSuiteConfig{
				transportName: transportName,
			}

			for _, serverOption := range tc.serverOptions {
				cfg.serverOptions = append(cfg.serverOptions, serverOption())
			}
			for _, clientOption := range tc.clientOptions {
				cfg.clientOptions = append(cfg.clientOptions, clientOption())
			}

			t.Run(fmt.Sprintf("%s/%s", transportName, tc.name), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
				defer func() {
					if promptListUpdater != nil {
						close(promptListUpdater.done)
					}
					if resourceListUpdater != nil {
						close(resourceListUpdater.done)
					}
					if resourceSubscriptionHandler != nil {
						close(resourceSubscriptionHandler.done)
					}
					if toolListUpdater != nil {
						close(toolListUpdater.done)
					}
					if logHandler != nil {
						close(logHandler.done)
					}
					promptListUpdater = nil
					resourceListUpdater = nil
					resourceSubscriptionHandler = nil
					toolListUpdater = nil
					logHandler = nil
				}()

				if tc.wantErr {
					if s.clientConnectErr == nil {
						t.Errorf("expected error, got nil")
					}
					return
				}
				if s.clientConnectErr != nil {
					t.Errorf("unexpected error: %v", s.clientConnectErr)
					return
				}

				srvInfo := s.mcpClient.ServerInfo()
				if srvInfo.Name != "test-server" {
					t.Errorf("expected server name test-server, got %s", srvInfo.Name)
				}
				if srvInfo.Version != "1.0" {
					t.Errorf("expected server version 1.0, got %s", srvInfo.Version)
				}
			}))
		}
	}
}

func TestUninitializedClient(t *testing.T) {
	// Create a client without connecting it
	client := mcp.NewClient(mcp.Info{
		Name:    "test-client",
		Version: "1.0",
	}, nil)

	t.Run("ListPrompts", func(t *testing.T) {
		_, err := client.ListPrompts(context.Background(), mcp.ListPromptsParams{})
		if err == nil || err.Error() != "client not initialized" {
			t.Errorf("expected 'client not initialized' error, got %v", err)
		}
	})

	t.Run("GetPrompt", func(t *testing.T) {
		_, err := client.GetPrompt(context.Background(), mcp.GetPromptParams{})
		if err == nil || err.Error() != "client not initialized" {
			t.Errorf("expected 'client not initialized' error, got %v", err)
		}
	})

	t.Run("CompletesPrompt", func(t *testing.T) {
		_, err := client.CompletesPrompt(context.Background(), mcp.CompletesCompletionParams{})
		if err == nil || err.Error() != "client not initialized" {
			t.Errorf("expected 'client not initialized' error, got %v", err)
		}
	})

	t.Run("ListResources", func(t *testing.T) {
		_, err := client.ListResources(context.Background(), mcp.ListResourcesParams{})
		if err == nil || err.Error() != "client not initialized" {
			t.Errorf("expected 'client not initialized' error, got %v", err)
		}
	})

	t.Run("ReadResource", func(t *testing.T) {
		_, err := client.ReadResource(context.Background(), mcp.ReadResourceParams{})
		if err == nil || err.Error() != "client not initialized" {
			t.Errorf("expected 'client not initialized' error, got %v", err)
		}
	})

	t.Run("ListResourceTemplates", func(t *testing.T) {
		_, err := client.ListResourceTemplates(context.Background(), mcp.ListResourceTemplatesParams{})
		if err == nil || err.Error() != "client not initialized" {
			t.Errorf("expected 'client not initialized' error, got %v", err)
		}
	})

	t.Run("SubscribeResource", func(t *testing.T) {
		err := client.SubscribeResource(context.Background(), mcp.SubscribeResourceParams{})
		if err == nil || err.Error() != "client not initialized" {
			t.Errorf("expected 'client not initialized' error, got %v", err)
		}
	})

	t.Run("ListTools", func(t *testing.T) {
		_, err := client.ListTools(context.Background(), mcp.ListToolsParams{})
		if err == nil || err.Error() != "client not initialized" {
			t.Errorf("expected 'client not initialized' error, got %v", err)
		}
	})

	t.Run("CallTool", func(t *testing.T) {
		_, err := client.CallTool(context.Background(), mcp.CallToolParams{})
		if err == nil || err.Error() != "client not initialized" {
			t.Errorf("expected 'client not initialized' error, got %v", err)
		}
	})

	t.Run("SetLogLevel", func(t *testing.T) {
		err := client.SetLogLevel(mcp.LogLevelDebug)
		if err == nil || err.Error() != "client not initialized" {
			t.Errorf("expected 'client not initialized' error, got %v", err)
		}
	})
}

func TestPrompt(t *testing.T) {
	for _, transportName := range []string{"SSE", "StdIO"} {
		promptServer := mockPromptServer{}
		progressListener := mockProgressListener{}

		cfg := testSuiteConfig{
			transportName: transportName,
			clientOptions: []mcp.ClientOption{
				mcp.WithProgressListener(&progressListener),
			},
		}

		t.Run(fmt.Sprintf("%s/UnsupportedPrompt", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.ListPrompts(context.Background(), mcp.ListPromptsParams{
				Cursor: "cursor",
				Meta: mcp.ParamsMeta{
					ProgressToken: "progressToken",
				},
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}

			_, err = s.mcpClient.GetPrompt(context.Background(), mcp.GetPromptParams{
				Name: "test-prompt",
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}

			_, err = s.mcpClient.CompletesPrompt(context.Background(), mcp.CompletesCompletionParams{
				Ref: mcp.CompletionRef{
					Type: mcp.CompletionRefPrompt,
					Name: "test-prompt",
				},
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}
		}))

		cfg.serverOptions = append(cfg.serverOptions, mcp.WithPromptServer(&promptServer))

		t.Run(fmt.Sprintf("%s/ListPrompts", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.ListPrompts(context.Background(), mcp.ListPromptsParams{
				Cursor: "cursor",
				Meta: mcp.ParamsMeta{
					ProgressToken: "progressToken",
				},
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if promptServer.listParams.Cursor != "cursor" {
				t.Errorf("expected cursor cursor, got %s", promptServer.listParams.Cursor)
			}

			time.Sleep(100 * time.Millisecond)

			progressListener.lock.Lock()
			defer progressListener.lock.Unlock()
			if progressListener.updateCount != 10 {
				t.Errorf("expected 10 progress params, got %d", progressListener.updateCount)
				return
			}
		}))

		t.Run(fmt.Sprintf("%s/GetPrompt", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.GetPrompt(context.Background(), mcp.GetPromptParams{
				Name: "test-prompt",
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if promptServer.getParams.Name != "test-prompt" {
				t.Errorf("expected prompt name test-prompt, got %s", promptServer.getParams.Name)
			}
		}))

		t.Run(fmt.Sprintf("%s/CompletesPrompt", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.CompletesPrompt(context.Background(), mcp.CompletesCompletionParams{
				Ref: mcp.CompletionRef{
					Type: mcp.CompletionRefPrompt,
					Name: "test-prompt",
				},
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if promptServer.completesParams.Ref.Name != "test-prompt" {
				t.Errorf("expected prompt name test-prompt, got %s", promptServer.completesParams.Ref.Name)
			}
		}))

		promptListUpdater := mockPromptListUpdater{
			ch:   make(chan struct{}),
			done: make(chan struct{}),
		}
		promptListWatcher := mockPromptListWatcher{}

		cfg.serverOptions = append(cfg.serverOptions, mcp.WithPromptListUpdater(promptListUpdater))
		cfg.clientOptions = append(cfg.clientOptions, mcp.WithPromptListWatcher(&promptListWatcher))

		t.Run(fmt.Sprintf("%s/UpdatePromptList", transportName), testSuiteCase(cfg, func(t *testing.T, _ *testSuite) {
			defer close(promptListUpdater.done)

			for i := 0; i < 5; i++ {
				promptListUpdater.ch <- struct{}{}
			}

			time.Sleep(100 * time.Millisecond)

			promptListWatcher.lock.Lock()
			defer promptListWatcher.lock.Unlock()
			if promptListWatcher.updateCount != 5 {
				t.Errorf("expected 5 prompt list updates, got %d", promptListWatcher.updateCount)
			}
		}))
	}
}

//nolint:gocognit
func TestResource(t *testing.T) {
	for _, transportName := range []string{"SSE", "StdIO"} {
		resourceServer := mockResourceServer{
			delayList: true,
		}

		cfg := testSuiteConfig{
			transportName: transportName,
		}

		t.Run(fmt.Sprintf("%s/UnsupportedResource", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.ListResources(context.Background(), mcp.ListResourcesParams{
				Cursor: "cursor",
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}

			_, err = s.mcpClient.ReadResource(context.Background(), mcp.ReadResourceParams{
				URI: "test://resource",
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}

			_, err = s.mcpClient.ListResourceTemplates(context.Background(), mcp.ListResourceTemplatesParams{
				Meta: mcp.ParamsMeta{
					ProgressToken: "progressToken",
				},
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}

			_, err = s.mcpClient.CompletesResourceTemplate(context.Background(), mcp.CompletesCompletionParams{
				Ref: mcp.CompletionRef{
					Type: mcp.CompletionRefResource,
					Name: "test-resource",
				},
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}

			err = s.mcpClient.SubscribeResource(context.Background(), mcp.SubscribeResourceParams{
				URI: "test://resource",
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}

			err = s.mcpClient.UnsubscribeResource(context.Background(), mcp.UnsubscribeResourceParams{
				URI: "test://resource",
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}
		}))

		cfg.serverOptions = append(cfg.serverOptions, mcp.WithResourceServer(&resourceServer))

		t.Run(fmt.Sprintf("%s/ListResourcesCancelled", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(100 * time.Millisecond)
				cancel()
			}()
			_, err := s.mcpClient.ListResources(ctx, mcp.ListResourcesParams{
				Cursor: "cursor",
			})
			if err == nil {
				t.Errorf("expected error, got nil")
				return
			}
		}))

		resourceServer.delayList = false
		cfg.serverOptions = append(cfg.serverOptions, mcp.WithResourceServer(&resourceServer))

		t.Run(fmt.Sprintf("%s/ListResources", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.ListResources(context.Background(), mcp.ListResourcesParams{
				Cursor: "cursor",
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if resourceServer.listParams.Cursor != "cursor" {
				t.Errorf("expected cursor cursor, got %s", resourceServer.listParams.Cursor)
			}
		}))

		t.Run(fmt.Sprintf("%s/ReadResources", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.ReadResource(context.Background(), mcp.ReadResourceParams{
				URI: "test://resource",
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if resourceServer.readParams.URI != "test://resource" {
				t.Errorf("expected cursor cursor, got %s", resourceServer.listParams.Cursor)
			}
		}))

		t.Run(fmt.Sprintf("%s/ListResourceTemplates", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.ListResourceTemplates(context.Background(), mcp.ListResourceTemplatesParams{
				Meta: mcp.ParamsMeta{
					ProgressToken: "progressToken",
				},
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if resourceServer.listTemplatesParams.Meta.ProgressToken != "progressToken" {
				t.Errorf("expected progressToken progressToken, got %s", resourceServer.listTemplatesParams.Meta.ProgressToken)
			}
		}))

		t.Run(fmt.Sprintf("%s/CompletesResourceTemplate", transportName),
			testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
				_, err := s.mcpClient.CompletesResourceTemplate(context.Background(), mcp.CompletesCompletionParams{
					Ref: mcp.CompletionRef{
						Type: mcp.CompletionRefResource,
						Name: "test-resource",
					},
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				if resourceServer.completesTemplateParams.Ref.Name != "test-resource" {
					t.Errorf("expected cursor cursor, got %s", resourceServer.listParams.Cursor)
				}
			}))

		resourceSubscriptionHandler := mockResourceSubscriptionHandler{
			ch:   make(chan string),
			done: make(chan struct{}),
		}

		resourceSubscriptionWatcher := mockResourceSubscribedWatcher{}

		cfg.serverOptions = append(cfg.serverOptions, mcp.WithResourceSubscriptionHandler(&resourceSubscriptionHandler))
		cfg.clientOptions = append(cfg.clientOptions, mcp.WithResourceSubscribedWatcher(&resourceSubscriptionWatcher))

		t.Run(fmt.Sprintf("%s/SubscribeResource", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			defer close(resourceSubscriptionHandler.done)

			err := s.mcpClient.SubscribeResource(context.Background(), mcp.SubscribeResourceParams{
				URI: "test://resource",
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if resourceSubscriptionHandler.subscribeParams.URI != "test://resource" {
				t.Errorf("expected URI test://resource, got %s", resourceSubscriptionHandler.subscribeParams.URI)
			}

			for i := 0; i < 5; i++ {
				resourceSubscriptionHandler.ch <- "test://resource"
			}

			time.Sleep(100 * time.Millisecond)

			resourceSubscriptionWatcher.lock.Lock()
			defer resourceSubscriptionWatcher.lock.Unlock()
			if resourceSubscriptionWatcher.updateCount != 5 {
				t.Errorf("expected 5 resource subscribed, got %d", resourceSubscriptionWatcher.updateCount)
			}

			err = s.mcpClient.UnsubscribeResource(context.Background(), mcp.UnsubscribeResourceParams{
				URI: "test://resource",
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if resourceSubscriptionHandler.unsubscribeParams.URI != "test://resource" {
				t.Errorf("expected URI test://resource, got %s", resourceSubscriptionHandler.unsubscribeParams.URI)
			}
		}))

		resourceListUpdater := mockResourceListUpdater{
			ch:   make(chan struct{}),
			done: make(chan struct{}),
		}
		resourceListWatcher := mockResourceListWatcher{}

		cfg.serverOptions = append(cfg.serverOptions, mcp.WithResourceListUpdater(resourceListUpdater))
		cfg.clientOptions = append(cfg.clientOptions, mcp.WithResourceListWatcher(&resourceListWatcher))

		t.Run(fmt.Sprintf("%s/UpdateResourceList", transportName), testSuiteCase(cfg, func(t *testing.T, _ *testSuite) {
			defer close(resourceListUpdater.done)

			for i := 0; i < 5; i++ {
				resourceListUpdater.ch <- struct{}{}
			}

			time.Sleep(100 * time.Millisecond)

			resourceListWatcher.lock.Lock()
			defer resourceListWatcher.lock.Unlock()
			if resourceListWatcher.updateCount != 5 {
				t.Errorf("expected 5 resource list updates, got %d", resourceListWatcher.updateCount)
			}
		}))
	}
}

func TestTool(t *testing.T) {
	for _, transportName := range []string{"SSE", "StdIO"} {
		toolServer := mockToolServer{
			requestRootsList: true,
		}
		rootsListHandler := mockRootsListHandler{}
		samplingHandler := mockSamplingHandler{}

		cfg := testSuiteConfig{
			transportName: transportName,
			serverOptions: []mcp.ServerOption{
				mcp.WithRequireRootsListClient(),
				mcp.WithRequireSamplingClient(),
			},
			clientOptions: []mcp.ClientOption{
				mcp.WithRootsListHandler(&rootsListHandler),
				mcp.WithSamplingHandler(&samplingHandler),
			},
		}

		t.Run(fmt.Sprintf("%s/UnsupportedTool", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.ListTools(context.Background(), mcp.ListToolsParams{
				Cursor: "cursor",
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}

			_, err = s.mcpClient.CallTool(context.Background(), mcp.CallToolParams{
				Name: "test-tool",
			})
			if err == nil {
				t.Errorf("expected error, got nil")
			}
		}))

		cfg.serverOptions = append(cfg.serverOptions, mcp.WithToolServer(&toolServer))

		t.Run(fmt.Sprintf("%s/ListTools", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.ListTools(context.Background(), mcp.ListToolsParams{
				Cursor: "cursor",
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if toolServer.listParams.Cursor != "cursor" {
				t.Errorf("expected cursor cursor, got %s", toolServer.listParams.Cursor)
			}

			time.Sleep(100 * time.Millisecond)

			if !rootsListHandler.called {
				t.Errorf("expected roots list handler to be called")
			}
		}))

		toolServer.requestRootsList = false
		toolServer.requestSampling = true
		cfg.serverOptions = append(cfg.serverOptions, mcp.WithToolServer(&toolServer))

		t.Run(fmt.Sprintf("%s/CallTool", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			_, err := s.mcpClient.CallTool(context.Background(), mcp.CallToolParams{
				Name: "test-tool",
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if toolServer.callParams.Name != "test-tool" {
				t.Errorf("expected tool name test-tool, got %s", toolServer.callParams.Name)
			}

			time.Sleep(100 * time.Millisecond)

			if !samplingHandler.called {
				t.Errorf("expected sampling handler to be called")
			}
		}))
	}
}

func TestRoot(t *testing.T) {
	for _, transportName := range []string{"SSE", "StdIO"} {
		rootsListUpdater := mockRootsListUpdater{
			ch: make(chan struct{}),
		}
		rootsListWatcher := mockRootsListWatcher{}

		cfg := testSuiteConfig{
			transportName: transportName,
			serverOptions: []mcp.ServerOption{
				mcp.WithRootsListWatcher(&rootsListWatcher),
			},
			clientOptions: []mcp.ClientOption{
				mcp.WithRootsListUpdater(rootsListUpdater),
			},
		}

		t.Run(fmt.Sprintf("%s/UpdateRootList", transportName), testSuiteCase(cfg, func(t *testing.T, _ *testSuite) {
			for i := 0; i < 5; i++ {
				rootsListUpdater.ch <- struct{}{}
			}

			time.Sleep(100 * time.Millisecond)

			rootsListWatcher.lock.Lock()
			defer rootsListWatcher.lock.Unlock()
			if rootsListWatcher.updateCount != 5 {
				t.Errorf("expected 5 root list updates, got %d", rootsListWatcher.updateCount)
			}
		}))
	}
}

func TestLog(t *testing.T) {
	for _, transportName := range []string{"SSE", "StdIO"} {
		handler := mockLogHandler{
			params: make(chan mcp.LogParams),
			done:   make(chan struct{}),
		}
		receiver := &mockLogReceiver{}

		cfg := testSuiteConfig{
			transportName: transportName,
			serverOptions: []mcp.ServerOption{
				mcp.WithLogHandler(&handler),
			},
			clientOptions: []mcp.ClientOption{
				mcp.WithLogReceiver(receiver),
			},
		}

		t.Run(fmt.Sprintf("%s/LogStream", transportName), testSuiteCase(cfg, func(t *testing.T, _ *testSuite) {
			defer close(handler.done)

			handler.level = mcp.LogLevelDebug
			for i := 0; i < 10; i++ {
				handler.params <- mcp.LogParams{}
			}

			time.Sleep(100 * time.Millisecond)

			receiver.lock.Lock()
			defer receiver.lock.Unlock()
			if receiver.updateCount != 10 {
				t.Errorf("expected 10 log params, got %d", receiver.updateCount)
			}
		}))

		// t.Run(fmt.Sprintf("%s/SetLogLevel", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
		// 	defer close(handler.done)
		// 	err := s.mcpClient.SetLogLevel(mcp.LogLevelError)
		// 	if err != nil {
		// 		t.Errorf("unexpected error: %v", err)
		// 	}
		//
		// 	time.Sleep(100 * time.Millisecond)
		//
		// 	handler.lock.Lock()
		// 	defer handler.lock.Unlock()
		// 	if handler.level != mcp.LogLevelError {
		// 		t.Errorf("expected log level %d, got %d", mcp.LogLevelError, handler.level)
		// 	}
		// }))
	}
}

// func TestServerPingAutoClose(t *testing.T) {
// 	for _, transportName := range []string{"SSE"} {
// 		clientCount := int64(0)
//
// 		cfg := testSuiteConfig{
// 			transportName: transportName,
// 			server:        &mockServer{},
// 			serverOptions: []mcp.ServerOption{
// 				mcp.WithServerPingTimeoutThreshold(3),
// 				mcp.WithServerPingInterval(1 * time.Second), // Very frequent pings
// 				mcp.WithServerOnClientConnected(func(string, mcp.Info) {
// 					atomic.AddInt64(&clientCount, 1)
// 				}),
// 				mcp.WithServerOnClientDisconnected(func(string) {
// 					atomic.AddInt64(&clientCount, -1)
// 				}),
// 			},
// 		}
//
// 		t.Run(fmt.Sprintf("%s/ConsecutiveFailedPing", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
// 			// At first, the client count should be one
// 			if atomic.LoadInt64(&clientCount) != 1 {
// 				t.Errorf("expected client count 1, got %d", atomic.LoadInt64(&clientCount))
// 			}
//
// 			// Disconnects client
// 			s.clientCancel()
//
// 			// Wait until failed ping exceeds threshold
// 			time.Sleep(5 * time.Second)
//
// 			log.Printf("Waiting for client count to be zero")
//
// 			// Should be zero as the client is disconnected
// 			if atomic.LoadInt64(&clientCount) != 0 {
// 				t.Errorf("expected client count 0, got %d", atomic.LoadInt64(&clientCount))
// 			}
// 		}))
// 	}
// }

func testSuiteCase(cfg testSuiteConfig, test func(*testing.T, *testSuite)) func(*testing.T) {
	return func(t *testing.T) {
		s := &testSuite{
			cfg: cfg,
		}
		s.setup()
		defer s.teardown(t)

		test(t, s)
	}
}

func setupSSE() (mcp.SSEServer, *mcp.SSEClient, *httptest.Server) {
	mux := http.NewServeMux()
	httpSrv := httptest.NewServer(mux)
	connectURL := fmt.Sprintf("%s/sse", httpSrv.URL)
	msgURL := fmt.Sprintf("%s/message", httpSrv.URL)

	srv := mcp.NewSSEServer(msgURL)

	mux.Handle("/sse", srv.HandleSSE())
	mux.Handle("/message", srv.HandleMessage())

	cli := mcp.NewSSEClient(connectURL, httpSrv.Client())

	return srv, cli, httpSrv
}

func setupStdIO() (mcp.StdIO, mcp.StdIO, *io.PipeReader, *io.PipeWriter, *io.PipeReader, *io.PipeWriter) {
	srvReader, srvWriter := io.Pipe()
	cliReader, cliWriter := io.Pipe()

	// server's output is client's input
	srvIO := mcp.NewStdIO(srvReader, cliWriter)
	// client's output is server's input
	cliIO := mcp.NewStdIO(cliReader, srvWriter)

	return srvIO, cliIO, srvReader, srvWriter, cliReader, cliWriter
}

func generateRandomJSON(approxSize int) json.RawMessage {
	res := make([]byte, 0, approxSize)

	// Make this JSON array
	res = append(res, []byte("[")...)
	for len(res) < approxSize {
		res = append(res, []byte(`{"dummyKey": "dummyVal"},`)...)
	}
	// Remove the last comma
	res = res[:len(res)-1]
	res = append(res, []byte("]")...)

	return res
}

func (t *testSuite) setup() {
	if t.cfg.transportName == "SSE" {
		t.serverTransport, t.clientTransport, t.httpServer = setupSSE()
	} else {
		t.serverTransport, t.clientTransport, t.srvIOReader, t.srvIOWriter, t.cliIOReader, t.cliIOWriter = setupStdIO()
	}

	clientCtx, clientCancel := context.WithCancel(context.Background())
	t.clientCancel = clientCancel

	t.mcpServer = mcp.NewServer(mcp.Info{
		Name:    "test-server",
		Version: "1.0",
	}, t.serverTransport, t.cfg.serverOptions...)

	go t.mcpServer.Serve()

	t.mcpClient = mcp.NewClient(mcp.Info{
		Name:    "test-client",
		Version: "1.0",
	}, t.clientTransport, t.cfg.clientOptions...)

	ready := make(chan struct{})
	errs := make(chan error)

	go func() {
		errs <- t.mcpClient.Connect(clientCtx, ready)
	}()

	timeout := time.After(50 * time.Millisecond)

	select {
	case <-timeout:
	case t.clientConnectErr = <-errs:
	}
	<-ready
}

func (t *testSuite) teardown(tt *testing.T) {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	err := t.mcpServer.Shutdown(shutdownCtx)
	if err != nil {
		tt.Errorf("failed to shutdown server: %v", err)
	}

	t.clientCancel()
	if t.cfg.transportName == "SSE" {
		t.httpServer.Close()
		return
	}

	_ = t.srvIOReader.Close()
	_ = t.srvIOWriter.Close()
	_ = t.cliIOReader.Close()
	_ = t.cliIOWriter.Close()
}
