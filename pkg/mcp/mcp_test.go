package mcp_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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
		wantSuccess       bool
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
			wantSuccess: true,
		},
		{
			name: "success with full capabilities",
			server: &mockServer{
				requireRootsListClient: true,
				requireSamplingClient:  true,
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
			server: &mockServer{
				requireRootsListClient: true,
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
			name:   "fail insufficient server capabilities",
			server: &mockServer{},
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

// func TestClientGetPrompt(t *testing.T) {
// 	tests := []struct {
// 		name          string
// 		promptName    string
// 		arguments     map[string]string
// 		progressToken mcp.MustString
// 		wantErr       bool
// 		wantResult    mcp.PromptResult
// 	}{
// 		{
// 			name:       "successful prompt request",
// 			promptName: "test-prompt",
// 			arguments: map[string]string{
// 				"test-arg": "test-value",
// 			},
// 			progressToken: "123",
// 			wantErr:       false,
// 			wantResult: mcp.PromptResult{
// 				Description: "Test Prompt",
// 				Messages: []mcp.PromptMessage{
// 					{
// 						Role: mcp.PromptRoleAssistant,
// 						Content: mcp.Content{
// 							Type: mcp.ContentTypeText,
// 							Text: "Test response message",
// 						},
// 					},
// 				},
// 			},
// 		},
// 		{
// 			name:       "empty prompt name",
// 			promptName: "",
// 			arguments: map[string]string{
// 				"test-arg": "test-value",
// 			},
// 			progressToken: "123",
// 			wantErr:       true,
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			serverTransport, clientTransport := setupStdIO()
//
// 			srv := &mockServer{}
// 			errsChan := make(chan error)
// 			ctx, cancel := context.WithCancel(context.Background())
// 			defer cancel()
//
// 			go mcp.Serve(ctx, srv, serverTransport, errsChan,
// 				mcp.WithPromptServer(mockPromptServer{}))
//
// 			cli := mcp.NewClient(mcp.Info{
// 				Name:    "test-client",
// 				Version: "1.0",
// 			}, clientTransport, mcp.ServerRequirement{
// 				PromptServer: true,
// 			})
//
// 			err := cli.Connect()
// 			if err != nil {
// 				t.Fatalf("failed to connect client: %v", err)
// 			}
// 			defer cli.Close()
//
// 			result, err := cli.GetPrompt(ctx, tt.promptName, tt.arguments, tt.progressToken)
//
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("GetPrompt() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
//
// 			if tt.wantErr {
// 				return
// 			}
//
// 			if result.Description != tt.wantResult.Description {
// 				t.Errorf("GetPrompt() description = %v, want %v",
// 					result.Description, tt.wantResult.Description)
// 			}
//
// 			if len(result.Messages) != len(tt.wantResult.Messages) {
// 				t.Errorf("GetPrompt() messages length = %v, want %v",
// 					len(result.Messages), len(tt.wantResult.Messages))
// 				return
// 			}
//
// 			msg := result.Messages[0]
// 			wantMsg := tt.wantResult.Messages[0]
//
// 			if msg.Role != wantMsg.Role {
// 				t.Errorf("GetPrompt() message role = %v, want %v",
// 					msg.Role, wantMsg.Role)
// 			}
//
// 			if msg.Content.Type != wantMsg.Content.Type {
// 				t.Errorf("GetPrompt() content type = %v, want %v",
// 					msg.Content.Type, wantMsg.Content.Type)
// 			}
//
// 			if msg.Content.Text != wantMsg.Content.Text {
// 				t.Errorf("GetPrompt() content text = %v, want %v",
// 					msg.Content.Text, wantMsg.Content.Text)
// 			}
// 		})
// 	}
// }

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

func getPageInfo(cursor string, pageSize, totalSize int) (int, int, string) {
	cursorInt, err := strconv.Atoi(cursor)
	if err != nil {
		cursorInt = 1
	}

	startIndex := (cursorInt - 1) * pageSize
	endIndex := startIndex + pageSize
	if endIndex > totalSize {
		endIndex = totalSize
	}

	countPage := totalSize / pageSize
	if totalSize%pageSize != 0 {
		countPage++
	}

	nextCursor := strconv.Itoa(cursorInt + 1)
	if cursorInt == countPage {
		nextCursor = ""
	}

	return startIndex, endIndex, nextCursor
}
