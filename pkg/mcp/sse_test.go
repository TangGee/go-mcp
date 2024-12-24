package mcp_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

func TestSSEConnect(t *testing.T) {
	type testCase struct {
		name        string
		srv         func() mcp.SSEServer
		cli         func(baseURL string, httpClient *http.Client) mcp.SSEClient
		wantSuccess bool
	}

	testCases := []testCase{
		{
			name: "success with no capabilities",
			srv: func() mcp.SSEServer {
				return mcp.NewSSEServer(&mockServer{})
			},
			cli: func(baseURL string, httpClient *http.Client) mcp.SSEClient {
				return mcp.NewSSEClient(&mockClient{}, baseURL, httpClient)
			},
			wantSuccess: true,
		},
		{
			name: "success with full capabilities",
			srv: func() mcp.SSEServer {
				return mcp.NewSSEServer(&mockServer{
					requireRootsListClient: true,
					requireSamplingClient:  true,
				}, mcp.WithPromptServer(mockPromptServer{}),
					mcp.WithResourceServer(mockResourceServer{}),
					mcp.WithToolServer(mockToolServer{}),
					mcp.WithLogHandler(mockLogHandler{}),
					mcp.WithRootsListWatcher(mockRootsListWatcher{}),
				)
			},
			cli: func(baseURL string, httpClient *http.Client) mcp.SSEClient {
				return mcp.NewSSEClient(&mockClient{
					requirePromptServer:   true,
					requireResourceServer: true,
					requireToolServer:     true,
				}, baseURL, httpClient, mcp.WithRootsListHandler(mockRootsListHandler{}),
					mcp.WithRootsListUpdater(mockRootsListUpdater{}),
					mcp.WithSamplingHandler(mockSamplingHandler{}),
					mcp.WithLogReceiver(mockLogReceiver{}),
				)
			},
			wantSuccess: true,
		},
		{
			name: "fail insufficient client capabilities",
			srv: func() mcp.SSEServer {
				return mcp.NewSSEServer(&mockServer{
					requireRootsListClient: true,
				}, mcp.WithPromptServer(mockPromptServer{}))
			},
			cli: func(baseURL string, httpClient *http.Client) mcp.SSEClient {
				return mcp.NewSSEClient(&mockClient{}, baseURL, httpClient)
			},
			wantSuccess: false,
		},
		{
			name: "fail insufficient server capabilities",
			srv: func() mcp.SSEServer {
				return mcp.NewSSEServer(&mockServer{
					requireRootsListClient: true,
					requireSamplingClient:  true,
				}, mcp.WithPromptServer(mockPromptServer{}),
					mcp.WithToolServer(mockToolServer{}),
					mcp.WithLogHandler(mockLogHandler{}),
					mcp.WithRootsListWatcher(mockRootsListWatcher{}),
				)
			},
			cli: func(baseURL string, httpClient *http.Client) mcp.SSEClient {
				return mcp.NewSSEClient(&mockClient{
					requirePromptServer:   true,
					requireResourceServer: true,
					requireToolServer:     true,
				}, baseURL, httpClient,
					mcp.WithRootsListHandler(mockRootsListHandler{}),
					mcp.WithRootsListUpdater(mockRootsListUpdater{}),
					mcp.WithSamplingHandler(mockSamplingHandler{}),
				)
			},
			wantSuccess: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := tc.srv()
			url, httpCli := startSSE(srv)
			cli := tc.cli(url, httpCli)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sessID, err := cli.Connect(ctx)

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

			if sessID == "" {
				t.Errorf("expected session ID, got empty string")
			}
		})
	}
}

func startSSE(srv mcp.SSEServer) (string, *http.Client) {
	mux := http.NewServeMux()
	httpSrv := httptest.NewServer(mux)

	baseURL := fmt.Sprintf("%s/sse", httpSrv.URL)
	msgBaseURL := fmt.Sprintf("%s/message", httpSrv.URL)
	mux.Handle("/sse", srv.HandleSSE(msgBaseURL))
	mux.Handle("/message", srv.HandleMessage())

	return baseURL, httpSrv.Client()
}
