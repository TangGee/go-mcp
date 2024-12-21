package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSSEClientConnect(t *testing.T) {
	tests := []struct {
		name        string
		serverSetup func() *httptest.Server
		wantErr     bool
	}{
		{
			name: "successful connection",
			serverSetup: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("http://test.com/messages?sessionID=test-session\n"))
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
				}))
			},
			wantErr: false,
		},
		{
			name: "server error",
			serverSetup: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.serverSetup()
			defer server.Close()

			client := NewSSEClient(&mockClient{}, server.URL, server.Client())

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			sessionID, err := client.Connect(ctx)
			if tt.wantErr {
				if err == nil {
					t.Error("Connect() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Connect() unexpected error: %v", err)
				}
				if sessionID == "" {
					t.Error("Connect() returned empty sessionID")
				}
			}
		})
	}
}
