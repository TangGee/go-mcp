package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MegaGrindStone/go-mcp"
)

type testSuite struct {
	cfg testSuiteConfig

	cancel          context.CancelFunc
	serverTransport mcp.ServerTransport
	clientTransport mcp.ClientTransport

	httpServer  *httptest.Server
	srvIOReader *io.PipeReader
	srvIOWriter *io.PipeWriter
	cliIOReader *io.PipeReader
	cliIOWriter *io.PipeWriter

	mcpClient        *mcp.Client
	clientConnectErr error
}

type testSuiteConfig struct {
	transportName string

	server        mcp.Server
	serverOptions []mcp.ServerOption

	clientOptions []mcp.ClientOption
}

func TestInitialize(t *testing.T) {
	type testCase struct {
		name          string
		server        mcp.Server
		serverOptions []mcp.ServerOption
		clientOptions []mcp.ClientOption
		wantErr       bool
	}

	testCases := []testCase{
		{
			name:          "success with no capabilities",
			server:        &mockServer{},
			serverOptions: []mcp.ServerOption{},
			clientOptions: []mcp.ClientOption{},
			wantErr:       false,
		},
		{
			name: "success with full capabilities",
			server: &mockServer{
				requireRootsListClient: true,
				requireSamplingClient:  true,
			},
			serverOptions: []mcp.ServerOption{
				mcp.WithPromptServer(&mockPromptServer{}),
				mcp.WithPromptListUpdater(mockPromptListUpdater{}),
				mcp.WithResourceServer(&mockResourceServer{}),
				mcp.WithResourceListUpdater(mockResourceListUpdater{}),
				mcp.WithResourceSubscriptionHandler(&mockResourceSubscriptionHandler{}),
				mcp.WithToolServer(&mockToolServer{}),
				mcp.WithToolListUpdater(mockToolListUpdater{}),
				mcp.WithLogHandler(&mockLogHandler{}),
				mcp.WithRootsListWatcher(&mockRootsListWatcher{}),
			},
			clientOptions: []mcp.ClientOption{
				mcp.WithPromptListWatcher(&mockPromptListWatcher{}),
				mcp.WithResourceListWatcher(&mockResourceListWatcher{}),
				mcp.WithResourceSubscribedWatcher(&mockResourceSubscribedWatcher{}),
				mcp.WithToolListWatcher(mockToolListWatcher{}),
				mcp.WithRootsListHandler(&mockRootsListHandler{}),
				mcp.WithRootsListUpdater(mockRootsListUpdater{}),
				mcp.WithSamplingHandler(&mockSamplingHandler{}),
				mcp.WithLogReceiver(&mockLogReceiver{}),
			},
			wantErr: false,
		},
		{
			name: "fail insufficient client capabilities",
			server: &mockServer{
				requireRootsListClient: true,
			},
			serverOptions: []mcp.ServerOption{
				mcp.WithPromptServer(&mockPromptServer{}),
			},
			clientOptions: []mcp.ClientOption{},
			wantErr:       true,
		},
	}

	for _, transportName := range []string{"SSE", "StdIO"} {
		for _, tc := range testCases {
			cfg := testSuiteConfig{
				transportName: transportName,
				server:        tc.server,
				serverOptions: tc.serverOptions,
				clientOptions: tc.clientOptions,
			}

			t.Run(fmt.Sprintf("%s/%s", transportName, tc.name), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
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
			server:        &mockServer{},
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
			ch: make(chan struct{}),
		}
		promptListWatcher := mockPromptListWatcher{}

		cfg.serverOptions = append(cfg.serverOptions, mcp.WithPromptListUpdater(promptListUpdater))
		cfg.clientOptions = append(cfg.clientOptions, mcp.WithPromptListWatcher(&promptListWatcher))

		t.Run(fmt.Sprintf("%s/UpdatePromptList", transportName), testSuiteCase(cfg, func(t *testing.T, _ *testSuite) {
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
			server:        &mockServer{},
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
			ch: make(chan string),
		}
		resourceSubscriptionWatcher := mockResourceSubscribedWatcher{}

		cfg.serverOptions = append(cfg.serverOptions, mcp.WithResourceSubscriptionHandler(&resourceSubscriptionHandler))
		cfg.clientOptions = append(cfg.clientOptions, mcp.WithResourceSubscribedWatcher(&resourceSubscriptionWatcher))

		t.Run(fmt.Sprintf("%s/SubscribeResource", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
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
			ch: make(chan struct{}),
		}
		resourceListWatcher := mockResourceListWatcher{}

		cfg.serverOptions = append(cfg.serverOptions, mcp.WithResourceListUpdater(resourceListUpdater))
		cfg.clientOptions = append(cfg.clientOptions, mcp.WithResourceListWatcher(&resourceListWatcher))

		t.Run(fmt.Sprintf("%s/UpdateResourceList", transportName), testSuiteCase(cfg, func(t *testing.T, _ *testSuite) {
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
			server: &mockServer{
				requireRootsListClient: true,
				requireSamplingClient:  true,
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
			server:        &mockServer{},
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
		}
		receiver := &mockLogReceiver{}

		cfg := testSuiteConfig{
			transportName: transportName,
			server:        &mockServer{},
			serverOptions: []mcp.ServerOption{
				mcp.WithLogHandler(&handler),
			},
			clientOptions: []mcp.ClientOption{
				mcp.WithLogReceiver(receiver),
			},
		}

		t.Run(fmt.Sprintf("%s/LogStream", transportName), testSuiteCase(cfg, func(t *testing.T, _ *testSuite) {
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

		t.Run(fmt.Sprintf("%s/SetLogLevel", transportName), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
			err := s.mcpClient.SetLogLevel(mcp.LogLevelError)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			time.Sleep(100 * time.Millisecond)

			handler.lock.Lock()
			defer handler.lock.Unlock()
			if handler.level != mcp.LogLevelError {
				t.Errorf("expected log level %d, got %d", mcp.LogLevelError, handler.level)
			}
		}))
	}
}

func testSuiteCase(cfg testSuiteConfig, test func(*testing.T, *testSuite)) func(*testing.T) {
	return func(t *testing.T) {
		s := &testSuite{
			cfg: cfg,
		}
		s.setup()
		defer s.teardown()

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

func (t *testSuite) setup() {
	if t.cfg.transportName == "SSE" {
		t.serverTransport, t.clientTransport, t.httpServer = setupSSE()
	} else {
		t.serverTransport, t.clientTransport, t.srvIOReader, t.srvIOWriter, t.cliIOReader, t.cliIOWriter = setupStdIO()
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel

	go mcp.Serve(ctx, t.cfg.server, t.serverTransport, t.cfg.serverOptions...)

	t.mcpClient = mcp.NewClient(mcp.Info{
		Name:    "test-client",
		Version: "1.0",
	}, t.clientTransport, t.cfg.clientOptions...)

	ready := make(chan struct{})
	errs := make(chan error)

	go func() {
		errs <- t.mcpClient.Connect(ctx, ready)
	}()

	timeout := time.After(50 * time.Millisecond)

	select {
	case <-timeout:
	case t.clientConnectErr = <-errs:
	}
	<-ready
}

func (t *testSuite) teardown() {
	t.cancel()
	if t.cfg.transportName == "SSE" {
		sseServer, _ := t.serverTransport.(mcp.SSEServer)
		t.httpServer.Close()
		sseServer.Close()
		return
	}

	_ = t.srvIOReader.Close()
	_ = t.srvIOWriter.Close()
	_ = t.cliIOReader.Close()
	_ = t.cliIOWriter.Close()
}

func generateRandomJSON(approxSize int) (json.RawMessage, error) {
	// Create a new random source with current time as seed
	rnd := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano()>>32)))

	// Build a random JSON object structure
	var builder strings.Builder
	builder.WriteString("{")

	// Track approximate size
	currentSize := 1 // Start with 1 for opening brace
	fieldCount := 0

	// Generate fields until we reach desired size
	for currentSize < approxSize-1 { // Leave room for closing brace
		if fieldCount > 0 {
			builder.WriteString(",")
			currentSize++
		}

		// Add field name
		fieldName := fmt.Sprintf("field%d", rnd.IntN(1000000))
		builder.WriteString(fmt.Sprintf(`"%s":`, fieldName))
		currentSize += len(fieldName) + 4 // +4 for quotes and colon

		// Determine remaining space
		remainingSpace := approxSize - currentSize - 1 // -1 for closing brace
		if remainingSpace <= 2 {                       // Not enough space for even a minimal value
			builder.WriteString(`""`)
			break
		}

		// Determine what type of value to generate
		valueType := rnd.IntN(5)

		switch valueType {
		case 0: // String
			maxStrLen := min(remainingSpace-2, 200) // -2 for quotes
			if maxStrLen < 1 {
				builder.WriteString(`""`)
				currentSize += 2
			} else {
				strLength := min(rnd.IntN(maxStrLen)+10, maxStrLen)
				randomStr := generateRandomString(rnd, strLength)
				builder.WriteString(fmt.Sprintf(`"%s"`, randomStr))
				currentSize += strLength + 2 // +2 for quotes
			}

		case 1: // Number
			num := rnd.Float64() * float64(rnd.IntN(10000))
			numStr := fmt.Sprintf("%.2f", num)
			if len(numStr) > remainingSpace {
				// Fallback to smaller value
				builder.WriteString("0")
				currentSize++
			} else {
				builder.WriteString(numStr)
				currentSize += len(numStr)
			}

		case 2: // Boolean
			if remainingSpace < 5 { // "false" needs 5 chars
				builder.WriteString("true")
				currentSize += 4
			} else {
				if rnd.IntN(2) == 0 {
					builder.WriteString("true")
					currentSize += 4
				} else {
					builder.WriteString("false")
					currentSize += 5
				}
			}

		case 3: // Nested array
			// Estimate size needed for a minimal array
			if remainingSpace < 10 { // Need at least "[]" plus some content
				builder.WriteString("[]")
				currentSize += 2
			} else {
				// Limit array size based on remaining space
				maxArraySize := min(20, remainingSpace/10)
				arraySize := rnd.IntN(maxArraySize) + 1
				arrayBytes := generateRandomArray(rnd, arraySize, remainingSpace-2) // -2 for []
				builder.WriteString(arrayBytes)
				currentSize += len(arrayBytes)
			}

		case 4: // Nested object
			// Estimate size needed for a minimal object
			if remainingSpace < 10 { // Need at least "{}" plus some content
				builder.WriteString("{}")
				currentSize += 2
			} else {
				nestedBytes := generateRandomObject(rnd, remainingSpace-2)
				builder.WriteString(nestedBytes)
				currentSize += len(nestedBytes)
			}
		}

		fieldCount++
	}

	builder.WriteString("}")
	jsonStr := builder.String()

	// Validate JSON by parsing it
	rawJSON := []byte(jsonStr)
	var result interface{}
	if err := json.Unmarshal(rawJSON, &result); err != nil {
		return nil, fmt.Errorf("generated invalid JSON: %w\n%s", err, jsonStr[:min(100, len(jsonStr))])
	}

	return json.RawMessage(rawJSON), nil
}

func generateRandomString(rnd *rand.Rand, length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rnd.IntN(len(chars))]
	}
	return string(result)
}

func generateRandomArray(rnd *rand.Rand, size int, maxSize int) string {
	var builder strings.Builder
	builder.WriteString("[")

	currentSize := 1 // [ character

	for i := 0; i < size && currentSize < maxSize-1; i++ { // -1 for closing ]
		if i > 0 {
			builder.WriteString(",")
			currentSize++
			if currentSize >= maxSize-1 {
				break
			}
		}

		remainingSpace := maxSize - currentSize - 1 // -1 for closing ]
		if remainingSpace <= 0 {
			break
		}

		valueType := rnd.IntN(3)

		switch valueType {
		case 0: // String
			maxStrLen := remainingSpace - 2 // -2 for quotes
			if maxStrLen < 1 {
				builder.WriteString(`""`)
				currentSize += 2
			} else {
				strLen := min(rnd.IntN(30)+5, maxStrLen)
				str := generateRandomString(rnd, strLen)
				strValue := fmt.Sprintf(`"%s"`, str)
				builder.WriteString(strValue)
				currentSize += len(strValue)
			}

		case 1: // Number
			num := rnd.Float64() * float64(rnd.IntN(1000))
			numStr := fmt.Sprintf("%.2f", num)
			if len(numStr) > remainingSpace {
				builder.WriteString("0")
				currentSize++
			} else {
				builder.WriteString(numStr)
				currentSize += len(numStr)
			}

		case 2: // Boolean
			if remainingSpace < 5 { // "false" needs 5 chars
				builder.WriteString("true")
				currentSize += 4
			} else {
				if rnd.IntN(2) == 0 {
					builder.WriteString("true")
					currentSize += 4
				} else {
					builder.WriteString("false")
					currentSize += 5
				}
			}
		}
	}

	builder.WriteString("]")
	return builder.String()
}

func generateRandomObject(rnd *rand.Rand, maxSize int) string {
	var builder strings.Builder
	builder.WriteString("{")

	numFields := min(rnd.IntN(10)+2, (maxSize-2)/10) // Estimate fields based on size
	currentSize := 1                                 // { character

	for i := 0; i < numFields && currentSize < maxSize-1; i++ { // -1 for closing }
		if i > 0 {
			builder.WriteString(",")
			currentSize++
			if currentSize >= maxSize-1 {
				break
			}
		}

		fieldName := fmt.Sprintf("n%d", rnd.IntN(1000))
		field := fmt.Sprintf(`"%s":`, fieldName)

		// Check if we have space for this field name
		if currentSize+len(field) >= maxSize-1 {
			break
		}

		builder.WriteString(field)
		currentSize += len(field)

		remainingSpace := maxSize - currentSize - 1 // -1 for closing }
		if remainingSpace <= 0 {
			break
		}

		valueType := rnd.IntN(3)

		switch valueType {
		case 0: // String
			maxStrLen := remainingSpace - 2 // -2 for quotes
			if maxStrLen < 1 {
				builder.WriteString(`""`)
				currentSize += 2
			} else {
				strLen := min(rnd.IntN(50)+10, maxStrLen)
				randomStr := generateRandomString(rnd, strLen)
				strValue := fmt.Sprintf(`"%s"`, randomStr)
				builder.WriteString(strValue)
				currentSize += len(strValue)
			}

		case 1: // Number
			num := rnd.Float64() * float64(rnd.IntN(1000))
			numStr := fmt.Sprintf("%.2f", num)
			if len(numStr) > remainingSpace {
				builder.WriteString("0")
				currentSize++
			} else {
				builder.WriteString(numStr)
				currentSize += len(numStr)
			}

		case 2: // Boolean
			if remainingSpace < 5 { // "false" needs 5 chars
				builder.WriteString("true")
				currentSize += 4
			} else {
				if rnd.IntN(2) == 0 {
					builder.WriteString("true")
					currentSize += 4
				} else {
					builder.WriteString("false")
					currentSize += 5
				}
			}
		}
	}

	builder.WriteString("}")
	return builder.String()
}
