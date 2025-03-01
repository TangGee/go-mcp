package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MegaGrindStone/go-mcp"
)

func TestSSEServerAndClient(t *testing.T) {
	// Create a test server
	mux := http.NewServeMux()
	testServer := httptest.NewServer(mux)

	server := mcp.NewSSEServer(testServer.URL + "/message")
	mux.Handle("/connect", server.HandleSSE())
	mux.Handle("/message", server.HandleMessage())
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			fmt.Printf("Server forced to shutdown: %v", err)
			return
		}

		testServer.Close()
	}()

	// Create client
	client := mcp.NewSSEClient(testServer.URL+"/connect", testServer.Client())

	// Start client session
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientSession, err := client.StartSession(ctx)
	if err != nil {
		t.Fatalf("failed to start session: %v", err)
	}
	defer clientSession.Stop()

	// Test sending message from server to client
	var receivedByClient mcp.JSONRPCMessage
	done := make(chan struct{})

	go func() {
		for msg := range clientSession.Messages() {
			receivedByClient = msg
			close(done)
			break
		}
	}()

	// Wait for first server session
	var serverSession mcp.Session
	sessions := make(chan mcp.Session, 1)
	go func() {
		for s := range server.Sessions() {
			sessions <- s
		}
	}()
	serverSession = <-sessions
	defer serverSession.Stop()

	// Send message from server to client
	serverMsg := mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		Method:  "test",
		Params:  json.RawMessage(`{"test": "hello"}`),
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()

	if err := serverSession.Send(sendCtx, serverMsg); err != nil {
		t.Fatalf("failed to send server message: %v", err)
	}

	// Wait for client to receive message
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for client to receive message")
	}

	if receivedByClient.Method != serverMsg.Method {
		t.Errorf("got method %q, want %q", receivedByClient.Method, serverMsg.Method)
	}

	// Test sending message from client to server
	clientMsg := mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		Method:  "response",
		Params:  json.RawMessage(`{"response": "world"}`),
	}

	var receivedByServer mcp.JSONRPCMessage
	serverDone := make(chan struct{})

	go func() {
		for msg := range serverSession.Messages() {
			receivedByServer = msg
			close(serverDone)
			break
		}
	}()

	if err := client.Send(ctx, clientMsg); err != nil {
		t.Fatalf("failed to send client message: %v", err)
	}

	// Wait for server to receive message
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for server to receive message")
	}

	if receivedByServer.Method != clientMsg.Method {
		t.Errorf("got method %q, want %q", receivedByServer.Method, clientMsg.Method)
	}
}

func TestSSEServerMultipleClients(t *testing.T) {
	// Create test server
	mux := http.NewServeMux()
	testServer := httptest.NewServer(mux)

	server := mcp.NewSSEServer(testServer.URL + "/message")
	mux.Handle("/connect", server.HandleSSE())
	mux.Handle("/message", server.HandleMessage())
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			fmt.Printf("Server forced to shutdown: %v", err)
			return
		}

		testServer.Close()
	}()

	// Listen for server sessions in goroutine
	sessionCount := int64(0)
	sessions := make(chan mcp.Session)
	go func() {
		for sess := range server.Sessions() {
			atomic.AddInt64(&sessionCount, 1)
			sessions <- sess
		}
		close(sessions)
	}()

	go func() {
		// Read all the messages from all the server sessions.
		ss := make([]mcp.Session, 0)
		for sess := range sessions {
			ss = append(ss, sess)
			go func(sess mcp.Session) {
				for msg := range sess.Messages() {
					t.Logf("received message: %s", msg.Method)
				}
			}(sess)
		}

		// Stop all the server sessions when we're done.
		for _, sess := range ss {
			sess.Stop()
		}
	}()

	// Create multiple concurrent clients to stress test session management
	for i := 0; i < 10; i++ {
		go func() {
			client := mcp.NewSSEClient(testServer.URL+"/connect", testServer.Client())

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			cliSession, err := client.StartSession(ctx)
			if err != nil {
				t.Logf("Failed to start session: %v", err)
				return
			}
			defer cliSession.Stop()

			// Concurrent message sending and receiving
			for msg := range cliSession.Messages() {
				t.Logf("Received message: %v", msg)
			}
		}()
	}

	time.Sleep(1 * time.Second)

	if atomic.LoadInt64(&sessionCount) != 10 {
		t.Errorf("Expected 10 sessions, got %d", sessionCount)
	}
}

func TestSSEConnectionNegativeCases(t *testing.T) {
	t.Run("Invalid Connection URL", func(t *testing.T) {
		client := mcp.NewSSEClient("http://non-existent-url-12345.local/connect", nil)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := client.StartSession(ctx)

		if err == nil {
			t.Fatal("Expected an error when connecting to invalid URL, got nil")
		}

		t.Logf("Connection error (expected): %v", err)
	})

	t.Run("Send Message Before Session", func(t *testing.T) {
		client := mcp.NewSSEClient("http://localhost:8080/connect", nil)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		msg := mcp.JSONRPCMessage{
			JSONRPC: mcp.JSONRPCVersion,
			Method:  "test",
			Params:  json.RawMessage(`{"test": "premature"}`),
		}

		err := client.Send(ctx, msg)

		if err == nil {
			t.Fatal("Expected an error when sending message before session, got nil")
		}

		t.Logf("Send message error (expected): %v", err)
	})

	t.Run("Invalid Message Format", func(t *testing.T) {
		mux := http.NewServeMux()
		testServer := httptest.NewServer(mux)

		server := mcp.NewSSEServer(testServer.URL + "/message")
		mux.Handle("/connect", server.HandleSSE())
		mux.Handle("/message", server.HandleMessage())
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			// Attempt graceful shutdown
			if err := server.Shutdown(ctx); err != nil {
				fmt.Printf("Server forced to shutdown: %v", err)
				return
			}

			testServer.Close()
		}()

		invalidMsg := []byte(`{invalid json}`)

		req, err := http.NewRequest(http.MethodPost, testServer.URL+"/message", bytes.NewBuffer(invalidMsg))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := testServer.Client().Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	t.Run("Session Timeout", func(t *testing.T) {
		mux := http.NewServeMux()
		testServer := httptest.NewServer(mux)

		server := mcp.NewSSEServer(testServer.URL + "/message")
		mux.Handle("/connect", server.HandleSSE())
		mux.Handle("/message", server.HandleMessage())
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			// Attempt graceful shutdown
			if err := server.Shutdown(ctx); err != nil {
				fmt.Printf("Server forced to shutdown: %v", err)
				return
			}

			testServer.Close()
		}()

		client := mcp.NewSSEClient(testServer.URL+"/connect", testServer.Client())

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Add a small delay to ensure context is cancelled
		time.Sleep(500 * time.Millisecond)

		_, err := client.StartSession(ctx)

		if err == nil {
			t.Fatal("Expected a timeout error, got nil")
		}

		t.Logf("Timeout error (expected): %v", err)
	})

	t.Run("Server Shutdown During Active Session", func(t *testing.T) {
		mux := http.NewServeMux()
		testServer := httptest.NewServer(mux)

		server := mcp.NewSSEServer(testServer.URL + "/message")
		mux.Handle("/connect", server.HandleSSE())
		mux.Handle("/message", server.HandleMessage())

		go func() {
			var session mcp.Session
			for sess := range server.Sessions() {
				session = sess
				go func(sess mcp.Session) {
					for msg := range sess.Messages() {
						t.Logf("received message: %s", msg.Method)
					}
				}(sess)
			}

			session.Stop()
		}()

		client := mcp.NewSSEClient(testServer.URL+"/connect", testServer.Client())

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		clientSession, err := client.StartSession(ctx)
		if err != nil {
			t.Fatalf("Failed to start session: %v", err)
		}
		defer clientSession.Stop()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer shutdownCancel()

		err = server.Shutdown(shutdownCtx)
		if err != nil {
			t.Fatalf("Failed to shutdown server: %v", err)
		}

		testServer.Close()

		msgReceived := false
		for range clientSession.Messages() {
			msgReceived = true
		}

		if msgReceived {
			t.Fatal("Expected no messages after server shutdown")
		}
	})
}

func TestSSEBidirectionalMessageFlow(t *testing.T) {
	// Create a test server
	mux := http.NewServeMux()
	testServer := httptest.NewServer(mux)

	server := mcp.NewSSEServer(testServer.URL + "/message")
	mux.Handle("/connect", server.HandleSSE())
	mux.Handle("/message", server.HandleMessage())
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			fmt.Printf("Server forced to shutdown: %v", err)
			return
		}

		testServer.Close()
	}()

	// Create client
	client := mcp.NewSSEClient(testServer.URL+"/connect", testServer.Client())

	// Start client session
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cliSession, err := client.StartSession(ctx)
	if err != nil {
		t.Fatalf("failed to start session: %v", err)
	}
	defer cliSession.Stop()

	// Prepare a series of messages for bidirectional communication
	testMessages := []mcp.JSONRPCMessage{
		{
			JSONRPC: mcp.JSONRPCVersion,
			Method:  "request1",
			Params:  json.RawMessage(`{"data": "first request"}`),
		},
		{
			JSONRPC: mcp.JSONRPCVersion,
			Method:  "request2",
			Params:  json.RawMessage(`{"data": "second request"}`),
		},
	}

	// Wait for first server session
	var srvSession mcp.Session
	sessions := make(chan mcp.Session, 1)
	go func() {
		for s := range server.Sessions() {
			sessions <- s
		}
	}()
	srvSession = <-sessions
	defer srvSession.Stop()

	// Channels to track message exchanges
	clientReceivedMsgs := make([]mcp.JSONRPCMessage, 0)
	serverReceivedMsgs := make([]mcp.JSONRPCMessage, 0)

	// Goroutine to receive messages on the client side
	clientMsgChan := make(chan mcp.JSONRPCMessage, len(testMessages))
	go func() {
		for msg := range cliSession.Messages() {
			clientMsgChan <- msg
		}
		close(clientMsgChan)
	}()

	// Goroutine to receive messages on the server side
	serverMsgChan := make(chan mcp.JSONRPCMessage, len(testMessages))
	go func() {
		for msg := range srvSession.Messages() {
			serverMsgChan <- msg
		}
		close(serverMsgChan)
	}()

	// Send messages in both directions
	for _, msg := range testMessages {
		sendCtx, sendCancel := context.WithTimeout(context.Background(), 1*time.Second)
		// Server to client
		if err := srvSession.Send(sendCtx, msg); err != nil {
			t.Fatalf("failed to send server message: %v", err)
		}
		sendCancel()

		// Client to server
		clientResponseMsg := mcp.JSONRPCMessage{
			JSONRPC: mcp.JSONRPCVersion,
			Method:  "response_" + msg.Method,
			Params:  json.RawMessage(`{"received": "` + msg.Method + `"}`),
		}
		if err := client.Send(ctx, clientResponseMsg); err != nil {
			t.Fatalf("failed to send client message: %v", err)
		}
	}

	// Collect received messages
	for i := 0; i < len(testMessages); i++ {
		select {
		case msg := <-clientMsgChan:
			clientReceivedMsgs = append(clientReceivedMsgs, msg)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for client message %d", i)
		}

		select {
		case msg := <-serverMsgChan:
			serverReceivedMsgs = append(serverReceivedMsgs, msg)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for server message %d", i)
		}
	}

	// Verify message flow
	if len(clientReceivedMsgs) != len(testMessages) {
		t.Errorf("client did not receive all messages. Got %d, want %d",
			len(clientReceivedMsgs), len(testMessages))
	}

	if len(serverReceivedMsgs) != len(testMessages) {
		t.Errorf("server did not receive all messages. Got %d, want %d",
			len(serverReceivedMsgs), len(testMessages))
	}

	for i, msg := range testMessages {
		if clientReceivedMsgs[i].Method != msg.Method {
			t.Errorf("client received wrong message. Got %s, want %s",
				clientReceivedMsgs[i].Method, msg.Method)
		}

		if serverReceivedMsgs[i].Method != "response_"+msg.Method {
			t.Errorf("server received wrong response. Got %s, want response_%s",
				serverReceivedMsgs[i].Method, msg.Method)
		}
	}
}

func TestSSELargeMessagePayload(t *testing.T) {
	// Create a test server
	mux := http.NewServeMux()
	testServer := httptest.NewServer(mux)

	server := mcp.NewSSEServer(testServer.URL + "/message")
	mux.Handle("/connect", server.HandleSSE())
	mux.Handle("/message", server.HandleMessage())
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			fmt.Printf("Server forced to shutdown: %v", err)
			return
		}

		testServer.Close()
	}()

	// Wait for client sessions
	var srvSession mcp.Session
	sessions := make(chan mcp.Session, 1)
	go func() {
		for s := range server.Sessions() {
			sessions <- s
		}
	}()

	// Generate a large payload with varying sizes
	payloadSizes := []int{
		1 * 1024,        // 1 KB
		100 * 1024,      // 100 KB
		1 * 1024 * 1024, // 1 MB
	}

	for _, size := range payloadSizes {
		t.Run(fmt.Sprintf("PayloadSize_%d", size), func(t *testing.T) {
			// Create client
			client := mcp.NewSSEClient(testServer.URL+"/connect", testServer.Client(),
				mcp.WithSSEClientMaxPayloadSize(1*1024*1024)) // 1 MB

			// Start client session
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cliSession, err := client.StartSession(ctx)
			if err != nil {
				t.Fatalf("failed to start session: %v", err)
			}
			defer cliSession.Stop()

			srvSession = <-sessions
			go func(sess mcp.Session) {
				for msg := range sess.Messages() {
					t.Logf("received message: %s", msg.Method)
				}
			}(srvSession)
			defer srvSession.Stop()

			// Generate random payload
			// This payload is required to be JSON message, instead of fully random bytes, because we want to test the
			// handling of the message payload in the server, not failing on unmarshalling the JSON.
			payload := generateRandomJSON(size)

			// Create a large message
			largeMsg := mcp.JSONRPCMessage{
				JSONRPC: mcp.JSONRPCVersion,
				Method:  "largePayload",
				Params:  payload,
			}

			// Channel to track message receipt
			receivedChan := make(chan mcp.JSONRPCMessage, 1)

			// Goroutine to receive message
			go func() {
				for msg := range cliSession.Messages() {
					receivedChan <- msg
					break
				}
			}()

			sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sendCancel()

			// Send large message from server to client
			if err := srvSession.Send(sendCtx, largeMsg); err != nil {
				t.Fatalf("failed to send large message: %v", err)
			}

			// Wait for message receipt
			select {
			case receivedMsg := <-receivedChan:
				// Verify message method
				if receivedMsg.Method != largeMsg.Method {
					t.Errorf("Incorrect method received. Got %s, want %s",
						receivedMsg.Method, largeMsg.Method)
				}

			case <-time.After(5 * time.Second):
				t.Fatalf("Timeout waiting for large message of size %d", size)
			}
		})
	}
}
