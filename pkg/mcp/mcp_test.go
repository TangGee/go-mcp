package mcp_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

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

	var transportName string
	for i := 0; i <= 1; i++ {
		if i == 0 {
			transportName = "SSE"
		} else {
			transportName = "StdIO"
		}
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%s", transportName, tc.name), func(t *testing.T) {
				// Create a new transports for each test case
				var serverTransport mcp.ServerTransport
				var clientTransport mcp.ClientTransport
				if i == 0 {
					var httpSrv *httptest.Server
					serverTransport, clientTransport, httpSrv = setupSSE()
					defer httpSrv.Close()
				} else {
					serverTransport, clientTransport = setupStdIO()
				}

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				srv := tc.server
				errsChan := make(chan error)

				go mcp.Serve(ctx, srv, serverTransport, errsChan, tc.serverOptions...)

				cliInfo := mcp.Info{
					Name:    "test-client",
					Version: "1.0",
				}
				cli := mcp.NewClient(cliInfo, clientTransport, tc.serverRequirement, tc.clientOptions...)
				err := cli.Connect()
				defer cli.Close()
				if tc.wantErr {
					if err == nil {
						t.Errorf("expected error, got nil")
					}
					return
				}
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
			})
		}
	}
}

func TestPrompt(t *testing.T) {
	type testCase struct {
		name     string
		testFunc func(*testing.T, *mcp.Client, *mockPromptServer)
	}

	testCases := []testCase{
		{
			name: "list",
			testFunc: func(t *testing.T, cli *mcp.Client, mockPs *mockPromptServer) {
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
			name: "get",
			testFunc: func(t *testing.T, cli *mcp.Client, mockPs *mockPromptServer) {
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
			name: "completes",
			testFunc: func(t *testing.T, cli *mcp.Client, mockPs *mockPromptServer) {
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
	}

	var transportName string
	for i := 0; i <= 1; i++ {
		if i == 0 {
			transportName = "SSE"
		} else {
			transportName = "StdIO"
		}
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%s", transportName, tc.name), func(t *testing.T) {
				// Create a new transports for each test case
				var serverTransport mcp.ServerTransport
				var clientTransport mcp.ClientTransport
				if i == 0 {
					var httpSrv *httptest.Server
					serverTransport, clientTransport, httpSrv = setupSSE()
					defer httpSrv.Close()
				} else {
					serverTransport, clientTransport = setupStdIO()
				}

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				srv := mockServer{}
				errsChan := make(chan error)
				mockPs := &mockPromptServer{}

				go mcp.Serve(ctx, srv, serverTransport, errsChan, mcp.WithPromptServer(mockPs))

				cliInfo := mcp.Info{
					Name:    "test-client",
					Version: "1.0",
				}
				cli := mcp.NewClient(cliInfo, clientTransport, mcp.ServerRequirement{
					PromptServer: true,
				})
				defer cli.Close()

				err := cli.Connect()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				tc.testFunc(t, cli, mockPs)
			})
		}
	}
}

func TestResource(t *testing.T) {
	type testCase struct {
		name     string
		testFunc func(*testing.T, *mcp.Client, *mockResourceServer)
	}

	testCases := []testCase{
		{
			name: "list",
			testFunc: func(t *testing.T, cli *mcp.Client, mockRs *mockResourceServer) {
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
			name: "read",
			testFunc: func(t *testing.T, cli *mcp.Client, mockRs *mockResourceServer) {
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
			name: "listTemplates",
			testFunc: func(t *testing.T, cli *mcp.Client, mockRs *mockResourceServer) {
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
			name: "completes",
			testFunc: func(t *testing.T, cli *mcp.Client, mockRs *mockResourceServer) {
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
			name: "subscribe",
			testFunc: func(t *testing.T, cli *mcp.Client, mockRs *mockResourceServer) {
				err := cli.SubscribeResource(context.Background(), mcp.SubscribeResourceParams{
					URI: "test://resource",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				if mockRs.subscribeParams.URI != "test://resource" {
					t.Errorf("expected URI test://resource, got %s", mockRs.subscribeParams.URI)
				}
			},
		},
		{
			name: "unsubscribe",
			testFunc: func(t *testing.T, cli *mcp.Client, mockRs *mockResourceServer) {
				err := cli.UnsubscribeResource(context.Background(), mcp.UnsubscribeResourceParams{
					URI: "test://resource",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				if mockRs.unsubscribeParams.URI != "test://resource" {
					t.Errorf("expected URI test://resource, got %s", mockRs.unsubscribeParams.URI)
				}
			},
		},
	}

	var transportName string
	for i := 0; i <= 1; i++ {
		if i == 0 {
			transportName = "SSE"
		} else {
			transportName = "StdIO"
		}
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%s", transportName, tc.name), func(t *testing.T) {
				// Create a new transports for each test case
				var serverTransport mcp.ServerTransport
				var clientTransport mcp.ClientTransport
				if i == 0 {
					var httpSrv *httptest.Server
					serverTransport, clientTransport, httpSrv = setupSSE()
					defer httpSrv.Close()
				} else {
					serverTransport, clientTransport = setupStdIO()
				}

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				srv := mockServer{}
				errsChan := make(chan error)
				mockRs := &mockResourceServer{}

				go mcp.Serve(ctx, srv, serverTransport, errsChan, mcp.WithResourceServer(mockRs))

				cliInfo := mcp.Info{
					Name:    "test-client",
					Version: "1.0",
				}
				cli := mcp.NewClient(cliInfo, clientTransport, mcp.ServerRequirement{
					ResourceServer: true,
				})
				defer cli.Close()

				err := cli.Connect()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				tc.testFunc(t, cli, mockRs)
			})
		}
	}
}

func TestTool(t *testing.T) {
	type testCase struct {
		name     string
		testFunc func(*testing.T, *mcp.Client, *mockToolServer)
	}

	testCases := []testCase{
		{
			name: "list",
			testFunc: func(t *testing.T, cli *mcp.Client, mockTs *mockToolServer) {
				_, err := cli.ListTools(context.Background(), mcp.ListToolsParams{
					Cursor: "cursor",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				if mockTs.listParams.Cursor != "cursor" {
					t.Errorf("expected cursor cursor, got %s", mockTs.listParams.Cursor)
				}
			},
		},
		{
			name: "call",
			testFunc: func(t *testing.T, cli *mcp.Client, mockTs *mockToolServer) {
				_, err := cli.CallTool(context.Background(), mcp.CallToolParams{
					Name: "test-tool",
				})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				if mockTs.callParams.Name != "test-tool" {
					t.Errorf("expected tool name test-tool, got %s", mockTs.callParams.Name)
				}
			},
		},
	}

	var transportName string
	for i := 0; i <= 1; i++ {
		if i == 0 {
			transportName = "SSE"
		} else {
			transportName = "StdIO"
		}
		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s/%s", transportName, tc.name), func(t *testing.T) {
				// Create a new transports for each test case
				var serverTransport mcp.ServerTransport
				var clientTransport mcp.ClientTransport
				if i == 0 {
					var httpSrv *httptest.Server
					serverTransport, clientTransport, httpSrv = setupSSE()
					defer httpSrv.Close()
				} else {
					serverTransport, clientTransport = setupStdIO()
				}

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				srv := mockServer{}
				errsChan := make(chan error)
				mockTS := &mockToolServer{}

				go mcp.Serve(ctx, srv, serverTransport, errsChan, mcp.WithToolServer(mockTS))

				cliInfo := mcp.Info{
					Name:    "test-client",
					Version: "1.0",
				}
				cli := mcp.NewClient(cliInfo, clientTransport, mcp.ServerRequirement{
					ToolServer: true,
				})
				defer cli.Close()

				err := cli.Connect()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				tc.testFunc(t, cli, mockTS)
			})
		}
	}
}

func setupSSE() (mcp.SSEServer, *mcp.SSEClient, *httptest.Server) {
	srv := mcp.NewSSEServer()

	mux := http.NewServeMux()
	httpSrv := httptest.NewServer(mux)

	baseURL := fmt.Sprintf("%s/sse", httpSrv.URL)
	msgBaseURL := fmt.Sprintf("%s/message", httpSrv.URL)
	mux.Handle("/sse", srv.HandleSSE(msgBaseURL))
	mux.Handle("/message", srv.HandleMessage())

	cli := mcp.NewSSEClient(baseURL, httpSrv.Client())

	return srv, cli, httpSrv
}

func setupStdIO() (mcp.StdIO, mcp.StdIO) {
	srvReader, srvWriter := io.Pipe()
	cliReader, cliWriter := io.Pipe()

	// client's output is server's input
	cliIO := mcp.NewStdIO(cliReader, srvWriter)
	// server's output is client's input
	srvIO := mcp.NewStdIO(srvReader, cliWriter)

	go srvIO.Start()
	go cliIO.Start()

	return srvIO, cliIO
}
