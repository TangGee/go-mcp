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
				mcp.WithResourceSubscribedUpdater(mockResourceSubscribedUpdater{}),
				mcp.WithToolServer(&mockToolServer{}),
				mcp.WithToolListUpdater(mockToolListUpdater{}),
				mcp.WithLogHandler(mockLogHandler{}),
				mcp.WithRootsListWatcher(mockRootsListWatcher{}),
			},
			clientOptions: []mcp.ClientOption{
				mcp.WithPromptListWatcher(mockPromptListWatcher{}),
				mcp.WithResourceListWatcher(mockResourceListWatcher{}),
				mcp.WithResourceSubscribedWatcher(mockResourceSubscribedWatcher{}),
				mcp.WithToolListWatcher(mockToolListWatcher{}),
				mcp.WithRootsListHandler(mockRootsListHandler{}),
				mcp.WithRootsListUpdater(mockRootsListUpdater{}),
				mcp.WithSamplingHandler(mockSamplingHandler{}),
				mcp.WithLogReceiver(mockLogReceiver{}),
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
				mcp.WithLogHandler(mockLogHandler{}),
				mcp.WithRootsListWatcher(mockRootsListWatcher{}),
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

//nolint:gocognit
func TestPrimitives(t *testing.T) {
	type testCase struct {
		name              string
		testType          string // "prompt", "resource", or "tool"
		serverRequirement mcp.ServerRequirement
		// interface{} will be type asserted to specific mock server
		testFunc func(*testing.T, *mcp.Client, interface{})
	}

	testCases := []testCase{
		{
			name:     "ListPrompts",
			testType: "prompt",
			serverRequirement: mcp.ServerRequirement{
				PromptServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockPs, _ := srv.(*mockPromptServer)
				_, err := cli.ListPrompts(context.Background(), mcp.ListPromptsParams{
					Cursor: "cursor",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockPs.listParams.Cursor != "cursor" {
					t.Errorf("expected cursor cursor, got %s", mockPs.listParams.Cursor)
				}
			},
		},
		{
			name:     "GetPrompt",
			testType: "prompt",
			serverRequirement: mcp.ServerRequirement{
				PromptServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockPs, _ := srv.(*mockPromptServer)
				_, err := cli.GetPrompt(context.Background(), mcp.GetPromptParams{
					Name: "test-prompt",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockPs.getParams.Name != "test-prompt" {
					t.Errorf("expected prompt name test-prompt, got %s", mockPs.getParams.Name)
				}
			},
		},
		{
			name:     "CompletesPrompt",
			testType: "prompt",
			serverRequirement: mcp.ServerRequirement{
				PromptServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockPs, _ := srv.(*mockPromptServer)
				_, err := cli.CompletesPrompt(context.Background(), mcp.CompletesCompletionParams{
					Ref: mcp.CompletionRef{
						Type: mcp.CompletionRefPrompt,
						Name: "test-prompt",
					},
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockPs.completesParams.Ref.Name != "test-prompt" {
					t.Errorf("expected prompt name test-prompt, got %s", mockPs.completesParams.Ref.Name)
				}
			},
		},
		{
			name:     "ListResources",
			testType: "resource",
			serverRequirement: mcp.ServerRequirement{
				ResourceServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockRs, _ := srv.(*mockResourceServer)
				_, err := cli.ListResources(context.Background(), mcp.ListResourcesParams{
					Cursor: "cursor",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockRs.listParams.Cursor != "cursor" {
					t.Errorf("expected cursor cursor, got %s", mockRs.listParams.Cursor)
				}
			},
		},
		{
			name:     "ReadResource",
			testType: "resource",
			serverRequirement: mcp.ServerRequirement{
				ResourceServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockRs, _ := srv.(*mockResourceServer)
				_, err := cli.ReadResource(context.Background(), mcp.ReadResourceParams{
					URI: "test://resource",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockRs.readParams.URI != "test://resource" {
					t.Errorf("expected URI test://resource, got %s", mockRs.readParams.URI)
				}
			},
		},
		{
			name:     "ListResourceTemplates",
			testType: "resource",
			serverRequirement: mcp.ServerRequirement{
				ResourceServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockRs, _ := srv.(*mockResourceServer)
				_, err := cli.ListResourceTemplates(context.Background(), mcp.ListResourceTemplatesParams{
					Meta: mcp.ParamsMeta{
						ProgressToken: "progressToken",
					},
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockRs.listTemplatesParams.Meta.ProgressToken != "progressToken" {
					t.Errorf("expected progressToken progressToken, got %s", mockRs.listTemplatesParams.Meta.ProgressToken)
				}
			},
		},
		{
			name:     "CompletesResourceTemplate",
			testType: "resource",
			serverRequirement: mcp.ServerRequirement{
				ResourceServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockRs, _ := srv.(*mockResourceServer)
				_, err := cli.CompletesResourceTemplate(context.Background(), mcp.CompletesCompletionParams{
					Ref: mcp.CompletionRef{
						Type: mcp.CompletionRefResource,
						Name: "test-resource",
					},
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockRs.completesTemplateParams.Ref.Name != "test-resource" {
					t.Errorf("expected resource name test-resource, got %s", mockRs.completesTemplateParams.Ref.Name)
				}
			},
		},
		{
			name:     "SubscribeResource",
			testType: "resource",
			serverRequirement: mcp.ServerRequirement{
				ResourceServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockRS, _ := srv.(*mockResourceServer)
				err := cli.SubscribeResource(context.Background(), mcp.SubscribeResourceParams{
					URI: "test://resource",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockRS.subscribeParams.URI != "test://resource" {
					t.Errorf("expected URI test://resource, got %s", mockRS.subscribeParams.URI)
				}
			},
		},
		{
			name:     "UnsubscribeResource",
			testType: "resource",
			serverRequirement: mcp.ServerRequirement{
				ResourceServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockRS, _ := srv.(*mockResourceServer)
				err := cli.UnsubscribeResource(context.Background(), mcp.UnsubscribeResourceParams{
					URI: "test://resource",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockRS.unsubscribeParams.URI != "test://resource" {
					t.Errorf("expected URI test://resource, got %s", mockRS.unsubscribeParams.URI)
				}
			},
		},
		{
			name:     "ListTools",
			testType: "tool",
			serverRequirement: mcp.ServerRequirement{
				ToolServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockTS, _ := srv.(*mockToolServer)
				_, err := cli.ListTools(context.Background(), mcp.ListToolsParams{
					Cursor: "cursor",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockTS.listParams.Cursor != "cursor" {
					t.Errorf("expected cursor cursor, got %s", mockTS.listParams.Cursor)
				}
			},
		},
		{
			name:     "CallTool",
			testType: "tool",
			serverRequirement: mcp.ServerRequirement{
				ToolServer: true,
			},
			testFunc: func(t *testing.T, cli *mcp.Client, srv interface{}) {
				mockTS, _ := srv.(*mockToolServer)
				_, err := cli.CallTool(context.Background(), mcp.CallToolParams{
					Name: "test-tool",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if mockTS.callParams.Name != "test-tool" {
					t.Errorf("expected tool name test-tool, got %s", mockTS.callParams.Name)
				}
			},
		},
	}

	for _, transport := range []string{"SSE", "StdIO"} {
		for _, tc := range testCases {
			srv := &mockServer{}
			var mockServer interface{}
			var serverOpt mcp.ServerOption

			switch tc.testType {
			case "prompt":
				mockPs := &mockPromptServer{}
				mockServer = mockPs
				serverOpt = mcp.WithPromptServer(mockPs)
			case "resource":
				mockRs := &mockResourceServer{}
				mockServer = mockRs
				serverOpt = mcp.WithResourceServer(mockRs)
			case "tool":
				mockTS := &mockToolServer{}
				mockServer = mockTS
				serverOpt = mcp.WithToolServer(mockTS)
			}

			cfg := testSuiteConfig{
				transportName:     transport,
				server:            srv,
				serverOptions:     []mcp.ServerOption{serverOpt},
				clientOptions:     []mcp.ClientOption{},
				serverRequirement: tc.serverRequirement,
			}
			t.Run(fmt.Sprintf("%s/%s", transport, tc.name), testSuiteCase(cfg, func(t *testing.T, s *testSuite) {
				if s.clientConnectErr != nil {
					t.Errorf("unexpected error: %v", s.clientConnectErr)
					return
				}

				tc.testFunc(t, s.mcpClient, mockServer)
			}))
		}
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
