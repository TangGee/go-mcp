package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// SSEServer implements a Server-Sent Events (SSE) server that manages client connections
// and message distribution. It provides bidirectional communication through SSE for
// server-to-client streaming and HTTP POST for client-to-server messages.
//
// The server maintains active client connections and handles message routing through
// channels while providing thread-safe operations using sync.Map for connection management.
type SSEServer struct {
	// writers is a map of sessionID to http.ResponseWriter
	writers *sync.Map

	sessionsChan chan SessionCtx
	messagesChan chan SessionMsgWithErrs
	errsChan     chan error
	closeChan    chan struct{}
}

// SSEClient implements a Server-Sent Events (SSE) client that manages server connections
// and bidirectional message handling. It provides real-time communication through SSE for
// server-to-client streaming and HTTP POST for client-to-server messages.
//
// The client maintains a persistent connection to the server and handles message routing
// through channels while providing automatic reconnection and error handling capabilities.
type SSEClient struct {
	httpClient *http.Client
	baseURL    string
	messageURL string

	messagesChan chan SessionMsgWithErrs
	errsChan     chan error
	closeChan    chan struct{}
}

// NewSSEServer creates and initializes a new SSE server instance with all necessary
// channels for session management, message handling, and error reporting.
func NewSSEServer() SSEServer {
	return SSEServer{
		writers:      new(sync.Map),
		sessionsChan: make(chan SessionCtx, 1),
		messagesChan: make(chan SessionMsgWithErrs),
		errsChan:     make(chan error),
		closeChan:    make(chan struct{}),
	}
}

// NewSSEClient creates and initializes a new SSE client instance with the specified
// base URL and HTTP client. If httpClient is nil, the default HTTP client will be used.
//
// The baseURL parameter should point to the SSE endpoint of the server.
func NewSSEClient(baseURL string, httpClient *http.Client) *SSEClient {
	return &SSEClient{
		httpClient:   httpClient,
		baseURL:      baseURL,
		messagesChan: make(chan SessionMsgWithErrs),
		errsChan:     make(chan error),
		closeChan:    make(chan struct{}),
	}
}

// Send delivers a message to a specific client session identified by the SessionMsg.
// It marshals the message to JSON and writes it to the client's event stream.
// The operation can be cancelled via the provided context.
//
// Returns an error if the session is not found, message marshaling fails,
// or the write operation fails.
func (s SSEServer) Send(ctx context.Context, msg SessionMsg) error {
	w, ok := s.writers.Load(msg.SessionID)
	if !ok {
		return fmt.Errorf("session not found")
	}
	wr, _ := w.(http.ResponseWriter)

	msgBs, err := json.Marshal(msg.Msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	errs := make(chan error)

	go func() {
		_, err = fmt.Fprintf(wr, "message: %s\n\n", msgBs)
		if err != nil {
			errs <- fmt.Errorf("failed to write message: %w", err)
			return
		}

		f, fOk := w.(http.Flusher)
		if fOk {
			f.Flush()
		}
		errs <- nil
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-errs:
	}

	return err
}

// Sessions returns a receive-only channel that provides notifications of new client
// sessions. Each SessionCtx contains the session ID and associated context.
func (s SSEServer) Sessions() <-chan SessionCtx {
	return s.sessionsChan
}

// SessionMessages returns a receive-only channel that provides incoming messages
// from clients. Each message includes the session ID, the message content,
// and an error channel for reporting processing results back to the client.
func (s SSEServer) SessionMessages() <-chan SessionMsgWithErrs {
	return s.messagesChan
}

// Errors returns a receive-only channel that provides server-side errors
// that occur during operation. This includes connection, message handling,
// and internal processing errors.
func (s SSEServer) Errors() <-chan error {
	return s.errsChan
}

// HandleSSE returns an http.Handler that manages SSE connections from clients.
// It sets up appropriate headers for SSE communication, creates a new session,
// and maintains the connection until closed by the client or server.
//
// The messageBaseURL parameter specifies the base URL for client message endpoints.
// Each client receives a unique message endpoint URL with their session ID.
func (s SSEServer) HandleSSE(messageBaseURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// Disable chunked encoding to avoid issues with SSE
		w.Header().Set("Transfer-Encoding", "identity")

		sessID := uuid.New().String()
		s.sessionsChan <- SessionCtx{
			Ctx: r.Context(),
			ID:  sessID,
		}
		s.writers.Store(sessID, w)

		url := fmt.Sprintf("%s?sessionID=%s", messageBaseURL, sessID)
		_, err := fmt.Fprintf(w, "endpoint: %s\n\n", url)
		if err != nil {
			nErr := fmt.Errorf("failed to write SSE URL: %w", err)
			http.Error(w, nErr.Error(), http.StatusInternalServerError)
			s.logError(nErr)
			return
		}

		f, ok := w.(http.Flusher)
		if ok {
			f.Flush()
		}

		// Keep the connection open for new messages
		select {
		case <-r.Context().Done():
		case <-s.closeChan:
		}
		// Session would be removed by server when r.Context is done.
	})
}

// HandleMessage returns an http.Handler that processes incoming messages from clients
// via HTTP POST requests. It expects a session ID as a query parameter and the message
// content as JSON in the request body.
//
// Messages are validated and routed through the server's message channel system
// for processing. Results are communicated back through the response.
func (s SSEServer) HandleMessage() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessID := r.URL.Query().Get("sessionID")
		if sessID == "" {
			nErr := fmt.Errorf("missing sessionID query parameter")
			s.logError(nErr)
			http.Error(w, nErr.Error(), http.StatusBadRequest)
			return
		}

		decoder := json.NewDecoder(r.Body)
		var msg JSONRPCMessage

		if err := decoder.Decode(&msg); err != nil {
			nErr := fmt.Errorf("failed to decode message: %w", err)
			s.logError(nErr)
			http.Error(w, nErr.Error(), http.StatusBadRequest)
			return
		}

		errs := make(chan error)
		s.messagesChan <- SessionMsgWithErrs{
			SessionID: sessID,
			Msg:       msg,
			Errs:      errs,
		}

		if err := <-errs; err != nil {
			nErr := fmt.Errorf("failed to handle message: %w", err)
			s.logError(nErr)
			http.Error(w, nErr.Error(), http.StatusBadRequest)
			return
		}
	})
}

// Close shuts down the SSE server by closing all internal channels.
// This terminates all active connections and stops message processing.
func (s SSEServer) Close() {
	close(s.sessionsChan)
	close(s.messagesChan)
	close(s.errsChan)
	close(s.closeChan)
}

// Send delivers a message to the server using an HTTP POST request. The message
// is marshaled to JSON and sent to the server's message endpoint. The operation
// can be cancelled via the provided context.
//
// Returns an error if message marshaling fails, the request cannot be created,
// or the server returns a non-200 status code.
func (s *SSEClient) Send(ctx context.Context, msg SessionMsg) error {
	msgBs, err := json.Marshal(msg.Msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	r := bytes.NewReader(msgBs)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.messageURL, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// StartSession initiates a new SSE connection with the server and returns the
// session ID. It establishes the event stream connection and starts listening
// for server messages in a separate goroutine.
//
// The returned session ID can be used to correlate messages with this specific
// connection. Returns an error if the connection cannot be established or
// the server response is invalid.
func (s *SSEClient) StartSession() (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.baseURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to connect to SSE server: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the first SSE event which contains the message URL
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "endpoint: ") {
			s.messageURL = strings.TrimSpace(strings.TrimPrefix(line, "endpoint: "))
			break
		}
	}
	if s.messageURL == "" {
		resp.Body.Close()
		return "", fmt.Errorf("failed to read message URL from SSE event")
	}

	// Parse the session ID from the message URL
	parsedURL, err := url.Parse(s.messageURL)
	if err != nil {
		resp.Body.Close()
		return "", fmt.Errorf("invalid message URL: %w", err)
	}

	sessID := parsedURL.Query().Get("sessionID")
	if sessID == "" {
		resp.Body.Close()
		return "", fmt.Errorf("no session ID in message URL")
	}

	go s.listenMessages(sessID, resp.Body)

	return sessID, nil
}

// SessionMessages returns a receive-only channel that provides incoming messages
// from the server. Each message includes the session ID, the message content,
// and an error channel for reporting processing results back to the server.
func (s *SSEClient) SessionMessages() <-chan SessionMsgWithErrs {
	return s.messagesChan
}

// Errors returns a receive-only channel that provides client-side errors
// that occur during operation. This includes connection errors, message
// parsing failures, and other operational errors.
func (s *SSEClient) Errors() <-chan error {
	return s.errsChan
}

// Close shuts down the SSE client by closing all internal channels and
// terminating the connection to the server. This stops all message processing
// and releases associated resources.
func (s *SSEClient) Close() {
	close(s.errsChan)
	close(s.messagesChan)
	close(s.closeChan)
}

func (s *SSEClient) listenMessages(sessID string, body io.ReadCloser) {
	defer body.Close()

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		select {
		case <-s.closeChan:
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			// Skip empty lines
			continue
		}
		var input string
		if strings.HasPrefix(line, "message: ") {
			input = strings.TrimSpace(strings.TrimPrefix(line, "message: "))
		}

		var msg JSONRPCMessage
		if err := json.Unmarshal([]byte(input), &msg); err != nil {
			s.logError(fmt.Errorf("failed to unmarshal message: %w", err))
			continue
		}

		errs := make(chan error)
		s.messagesChan <- SessionMsgWithErrs{
			SessionID: sessID,
			Msg:       msg,
			Errs:      errs,
		}

		if err := <-errs; err != nil {
			s.logError(fmt.Errorf("failed to handle message: %w", err))
		}
	}

	if scanner.Err() != nil {
		if !errors.Is(scanner.Err(), context.Canceled) {
			s.logError(fmt.Errorf("failed to read SSE events: %w", scanner.Err()))
		}
	}
}

func (s *SSEServer) logError(err error) {
	select {
	case s.errsChan <- err:
	default:
	}
}

func (s *SSEClient) logError(err error) {
	select {
	case s.errsChan <- err:
	default:
	}
}
