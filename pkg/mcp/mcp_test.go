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
		server            func() mcp.Server
		serverOptions     []mcp.ServerOption
		clientOptions     []mcp.ClientOption
		serverRequirement mcp.ServerRequirement
		wantSuccess       bool
	}

	testCases := []testCase{
		{
			name: "success with no capabilities",
			server: func() mcp.Server {
				return &mockServer{}
			},
			serverOptions: []mcp.ServerOption{},
			clientOptions: []mcp.ClientOption{},
			serverRequirement: mcp.ServerRequirement{
				PromptServer:   false,
				ResourceServer: false,
				ToolServer:     false,
			},
			wantSuccess: true,
		},
		{
			name: "success with full capabilities",
			server: func() mcp.Server {
				return &mockServer{
					requireRootsListClient: true,
					requireSamplingClient:  true,
				}
			},
			serverOptions: []mcp.ServerOption{
				mcp.WithPromptServer(mockPromptServer{}),
				mcp.WithPromptListUpdater(mockPromptListUpdater{}),
				mcp.WithResourceServer(mockResourceServer{}),
				mcp.WithResourceListUpdater(mockResourceListUpdater{}),
				mcp.WithResourceSubscribedUpdater(mockResourceSubscribedUpdater{}),
				mcp.WithToolServer(mockToolServer{}),
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
			wantSuccess: true,
		},
		{
			name: "fail insufficient client capabilities",
			server: func() mcp.Server {
				return &mockServer{
					requireRootsListClient: true,
				}
			},
			serverOptions: []mcp.ServerOption{
				mcp.WithPromptServer(mockPromptServer{}),
			},
			clientOptions: []mcp.ClientOption{},
			serverRequirement: mcp.ServerRequirement{
				PromptServer: true,
			},
			wantSuccess: false,
		},
		{
			name: "fail insufficient server capabilities",
			server: func() mcp.Server {
				return &mockServer{}
			},
			serverOptions: []mcp.ServerOption{
				mcp.WithPromptServer(mockPromptServer{}),
				mcp.WithToolServer(mockToolServer{}),
				mcp.WithLogHandler(mockLogHandler{}),
				mcp.WithRootsListWatcher(mockRootsListWatcher{}),
			},
			clientOptions: []mcp.ClientOption{},
			serverRequirement: mcp.ServerRequirement{
				ResourceServer: true,
			},
			wantSuccess: false,
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
					serverTransport, clientTransport = setupSSE()
				} else {
					serverTransport, clientTransport = setupStdIO()
				}

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				srv := tc.server()
				errsChan := make(chan error)

				go mcp.Serve(ctx, srv, serverTransport, errsChan, tc.serverOptions...)

				cliInfo := mcp.Info{
					Name:    "test-client",
					Version: "1.0",
				}
				cli := mcp.NewClient(cliInfo, clientTransport, tc.serverRequirement, tc.clientOptions...)
				err := cli.Connect()
				defer cli.Close()
				if !tc.wantSuccess {
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

func setupSSE() (mcp.SSEServer, *mcp.SSEClient) {
	srv := mcp.NewSSEServer()

	mux := http.NewServeMux()
	httpSrv := httptest.NewServer(mux)

	baseURL := fmt.Sprintf("%s/sse", httpSrv.URL)
	msgBaseURL := fmt.Sprintf("%s/message", httpSrv.URL)
	mux.Handle("/sse", srv.HandleSSE(msgBaseURL))
	mux.Handle("/message", srv.HandleMessage())

	cli := mcp.NewSSEClient(baseURL, httpSrv.Client())

	return srv, cli
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
