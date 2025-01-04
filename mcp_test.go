package mcp_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MegaGrindStone/go-mcp"
)

type testSuite struct {
	cfg testSuiteConfig

	cancel          context.CancelFunc
	serverTransport mcp.ServerTransport
	clientTransport mcp.ClientTransport
	httpServer      *httptest.Server

	mcpClient        *mcp.Client
	clientConnectErr error
}

type testSuiteConfig struct {
	transportName string

	server        mcp.Server
	serverOptions []mcp.ServerOption

	clientOptions     []mcp.ClientOption
	serverRequirement mcp.ServerRequirement
}

func TestInitialize(t *testing.T) {
	type testCase struct {
		name              string
		server            mcp.Server
		serverOptions     []mcp.ServerOption
		clientOptions     []mcp.ClientOption
		serverRequirement mcp.ServerRequirement
		wantErr           bool
	}

	testCases := []testCase{
		{
			name:          "success with no capabilities",
			server:        &mockServer{},
			serverOptions: []mcp.ServerOption{},
			clientOptions: []mcp.ClientOption{},
			serverRequirement: mcp.ServerRequirement{
				PromptServer:   false,
				ResourceServer: false,
				ToolServer:     false,
			},
			wantErr: false,
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
			serverRequirement: mcp.ServerRequirement{
				PromptServer:   true,
				ResourceServer: true,
				ToolServer:     true,
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
			serverRequirement: mcp.ServerRequirement{
				PromptServer: true,
			},
			wantErr: true,
		},
		{
			name:   "fail insufficient server capabilities",
			server: &mockServer{},
			serverOptions: []mcp.ServerOption{
				mcp.WithPromptServer(&mockPromptServer{}),
				mcp.WithToolServer(&mockToolServer{}),
				mcp.WithLogHandler(&mockLogHandler{}),
				mcp.WithRootsListWatcher(&mockRootsListWatcher{}),
			},
			clientOptions: []mcp.ClientOption{},
			serverRequirement: mcp.ServerRequirement{
				ResourceServer: true,
			},
			wantErr: true,
		},
	}

	for _, transportName := range []string{"SSE", "StdIO"} {
		for _, tc := range testCases {
			cfg := testSuiteConfig{
				transportName:     transportName,
				server:            tc.server,
				serverOptions:     tc.serverOptions,
				clientOptions:     tc.clientOptions,
				serverRequirement: tc.serverRequirement,
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
			}))
		}
	}
}

func TestPrompt(t *testing.T) {
	for _, transportName := range []string{"SSE", "StdIO"} {
		promptServer := mockPromptServer{}
		progressListener := mockProgressListener{}

		cfg := testSuiteConfig{
			transportName: transportName,
			server:        &mockServer{},
			serverOptions: []mcp.ServerOption{
				mcp.WithPromptServer(&promptServer),
			},
			clientOptions: []mcp.ClientOption{
				mcp.WithProgressListener(&progressListener),
			},
			serverRequirement: mcp.ServerRequirement{
				PromptServer: true,
			},
		}

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

func TestResource(t *testing.T) {
	for _, transportName := range []string{"SSE", "StdIO"} {
		resourceServer := mockResourceServer{
			delayList: true,
		}

		cfg := testSuiteConfig{
			transportName: transportName,
			server:        &mockServer{},
			serverOptions: []mcp.ServerOption{
				mcp.WithResourceServer(&resourceServer),
			},
			serverRequirement: mcp.ServerRequirement{
				ResourceServer: true,
			},
		}

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
			serverOptions: []mcp.ServerOption{
				mcp.WithToolServer(&toolServer),
			},
			clientOptions: []mcp.ClientOption{
				mcp.WithRootsListHandler(&rootsListHandler),
				mcp.WithSamplingHandler(&samplingHandler),
			},
			serverRequirement: mcp.ServerRequirement{
				ToolServer: true,
			},
		}

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
			serverRequirement: mcp.ServerRequirement{},
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
			serverRequirement: mcp.ServerRequirement{
				PromptServer:   false,
				ResourceServer: false,
				ToolServer:     false,
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

func setupStdIO() (mcp.StdIO, mcp.StdIO) {
	srvReader, srvWriter := io.Pipe()
	cliReader, cliWriter := io.Pipe()

	// server's output is client's input
	srvIO := mcp.NewStdIO(srvReader, cliWriter)
	// client's output is server's input
	cliIO := mcp.NewStdIO(cliReader, srvWriter)

	return srvIO, cliIO
}

func (t *testSuite) setup() {
	if t.cfg.transportName == "SSE" {
		t.serverTransport, t.clientTransport, t.httpServer = setupSSE()
	} else {
		t.serverTransport, t.clientTransport = setupStdIO()
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel

	go mcp.Serve(ctx, t.cfg.server, t.serverTransport, t.cfg.serverOptions...)

	t.mcpClient = mcp.NewClient(mcp.Info{
		Name:    "test-client",
		Version: "1.0",
	}, t.clientTransport, t.cfg.serverRequirement, t.cfg.clientOptions...)

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
	srvIO, _ := t.serverTransport.(mcp.StdIO)
	cliIO, _ := t.clientTransport.(mcp.StdIO)
	srvIO.Close()
	cliIO.Close()
}
