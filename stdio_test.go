package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/MegaGrindStone/go-mcp"
)

func TestStdIOBidirectionalMessageFlow(t *testing.T) {
	// Create buffered pipes to simulate stdin/stdout
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	// Create StdIO instances
	serverTransport := mcp.NewStdIO(serverReader, serverWriter)
	clientTransport := mcp.NewStdIO(clientReader, clientWriter)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Prepare test messages
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

	// Channels to track message exchanges
	clientReceivedMsgs := make([]mcp.JSONRPCMessage, 0)
	serverReceivedMsgs := make([]mcp.JSONRPCMessage, 0)

	// Prepare client session
	ready := make(chan error, 1)
	clientMsgs, err := clientTransport.StartSession(ctx, ready)
	if err != nil {
		t.Fatalf("failed to start client session: %v", err)
	}

	// Wait for connection readiness
	if err := <-ready; err != nil {
		t.Fatalf("connection not ready: %v", err)
	}

	// Get server session
	var serverSession mcp.Session
	for s := range serverTransport.Sessions() {
		serverSession = s
		break
	}

	// Synchronization for message tracking
	var wg sync.WaitGroup
	wg.Add(2)

	// Receive messages on client side
	go func() {
		defer wg.Done()
		for msg := range clientMsgs {
			clientReceivedMsgs = append(clientReceivedMsgs, msg)
			if len(clientReceivedMsgs) == len(testMessages) {
				return
			}
		}
	}()

	// Receive messages on server side
	go func() {
		defer wg.Done()
		for msg := range serverSession.Messages() {
			serverReceivedMsgs = append(serverReceivedMsgs, msg)
			if len(serverReceivedMsgs) == len(testMessages) {
				return
			}
		}
	}()

	// Send messages in both directions
	for _, msg := range testMessages {
		// Server to client
		if err := serverSession.Send(ctx, msg); err != nil {
			t.Fatalf("failed to send server message: %v", err)
		}

		// Client to server
		clientResponseMsg := mcp.JSONRPCMessage{
			JSONRPC: mcp.JSONRPCVersion,
			Method:  "response_" + msg.Method,
			Params:  json.RawMessage(`{"received": "` + msg.Method + `"}`),
		}
		if err := clientTransport.Send(ctx, clientResponseMsg); err != nil {
			t.Fatalf("failed to send client message: %v", err)
		}
	}

	// Wait for message collection
	wg.Wait()

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

func TestStdIOContextCancellation(t *testing.T) {
	// Create buffered pipes to simulate stdin/stdout
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	// Create StdIO instances
	serverTransport := mcp.NewStdIO(serverReader, serverWriter)
	_ = mcp.NewStdIO(clientReader, clientWriter)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Attempt to send a message with a context that will timeout quickly
	msg := mcp.JSONRPCMessage{
		JSONRPC: mcp.JSONRPCVersion,
		Method:  "test_cancellation",
		Params:  json.RawMessage(`{"test": "cancel"}`),
	}

	// Get first server session
	var serverSession mcp.Session
	for s := range serverTransport.Sessions() {
		serverSession = s
		break
	}

	// Wait a bit to ensure context times out
	time.Sleep(200 * time.Millisecond)

	// Attempt to send message, expect context cancellation error
	err := serverSession.Send(ctx, msg)
	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}

func TestStdIOLargeMessagePayload(t *testing.T) {
	// Create buffered pipes to simulate stdin/stdout
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	// Create StdIO instances
	serverTransport := mcp.NewStdIO(serverReader, serverWriter)
	clientTransport := mcp.NewStdIO(clientReader, clientWriter)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Payload sizes to test
	payloadSizes := []int{
		1 * 1024,        // 1 KB
		100 * 1024,      // 100 KB
		1 * 1024 * 1024, // 1 MB
	}

	for _, size := range payloadSizes {
		t.Run(fmt.Sprintf("PayloadSize_%d", size), func(t *testing.T) {
			// Generate random JSON payload
			// This payload is required to be JSON message, instead of fully random bytes, because we want to test the
			// handling of the message payload in the server, not failing on unmarshalling the JSON.
			payload := generateRandomJSON(size)

			// Create a large message
			largeMsg := mcp.JSONRPCMessage{
				JSONRPC: mcp.JSONRPCVersion,
				Method:  "largePayload",
				Params:  payload,
			}

			// Prepare client session
			ready := make(chan error, 1)
			clientMsgs, err := clientTransport.StartSession(ctx, ready)
			if err != nil {
				t.Fatalf("failed to start client session: %v", err)
			}

			// Wait for connection readiness
			if err := <-ready; err != nil {
				t.Fatalf("connection not ready: %v", err)
			}

			// Get server session
			var serverSession mcp.Session
			for s := range serverTransport.Sessions() {
				serverSession = s
				break
			}

			// Channel to track message receipt
			receivedChan := make(chan mcp.JSONRPCMessage, 1)

			// Goroutine to receive message
			go func() {
				for msg := range clientMsgs {
					receivedChan <- msg
					break
				}
			}()

			// Send large message from server to client
			if err := serverSession.Send(ctx, largeMsg); err != nil {
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
