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
					var sseServer mcp.SSEServer
					sseServer, clientTransport, httpSrv = setupSSE()
					defer sseServer.Close()
					defer httpSrv.Close()
					serverTransport = sseServer
				} else {
					srvIO, cliIO := setupStdIO()
					defer srvIO.Close()
					defer cliIO.Close()
					serverTransport = srvIO
					clientTransport = cliIO
				}

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				srv := tc.server

				go mcp.Serve(ctx, srv, serverTransport, tc.serverOptions...)

				cliInfo := mcp.Info{
					Name:    "test-client",
					Version: "1.0",
				}
				cli := mcp.NewClient(cliInfo, clientTransport, tc.serverRequirement, tc.clientOptions...)

				ready := make(chan struct{})
				errs := make(chan error)

				go func() {
					errs <- cli.Connect(ctx, ready)
				}()

				var err error
				timeout := time.After(100 * time.Millisecond)

				select {
				case <-timeout:
				case err = <-errs:
				}
				<-ready

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

//nolint:gocognit,gocyclo,cyclop // Avoid repetition
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
			t.Run(fmt.Sprintf("%s/%s", transport, tc.name), func(t *testing.T) {
				var serverTransport mcp.ServerTransport
				var clientTransport mcp.ClientTransport
				if transport == "SSE" {
					var httpSrv *httptest.Server
					var sseServer mcp.SSEServer
					sseServer, clientTransport, httpSrv = setupSSE()
					defer sseServer.Close()
					defer httpSrv.Close()
					serverTransport = sseServer
				} else {
					srvIO, cliIO := setupStdIO()
					defer srvIO.Close()
					defer cliIO.Close()
					serverTransport = srvIO
					clientTransport = cliIO
				}

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

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

				go mcp.Serve(ctx, srv, serverTransport, serverOpt)

				cli := mcp.NewClient(mcp.Info{
					Name:    "test-client",
					Version: "1.0",
				}, clientTransport, tc.serverRequirement)

				ready := make(chan struct{})
				errs := make(chan error)

				go func() {
					errs <- cli.Connect(ctx, ready)
				}()

				var err error
				timeout := time.After(100 * time.Millisecond)

				select {
				case <-timeout:
				case err = <-errs:
				}
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				<-ready

				tc.testFunc(t, cli, mockServer)
			})
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
