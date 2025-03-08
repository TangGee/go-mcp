package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/tmaxmax/go-sse"
)

// SSEServer implements a framework-agnostic Server-Sent Events (SSE) server for managing
// bidirectional client communication. It handles server-to-client streaming through SSE
// and client-to-server messaging via HTTP POST endpoints.
//
// The server provides connection management, message distribution, and session tracking
// capabilities through its HandleSSE and HandleMessage http.Handlers. These handlers can
// be integrated with any HTTP framework.
//
// Instances should be created using NewSSEServer and properly shut down using Shutdown when
// no longer needed.
type SSEServer struct {
	messageURL string
	logger     *slog.Logger

	sessions         chan sseServerSession
	removedSessions  chan string
	receivedMessages chan sseSessionMessage

	done   chan struct{}
	closed chan struct{}
}

// SSEClient implements a Server-Sent Events (SSE) client that manages server connections
// and bidirectional message handling. It provides real-time communication through SSE for
// server-to-client streaming and HTTP POST for client-to-server messages.
// Instances should be created using NewSSEClient.
type SSEClient struct {
	httpClient *http.Client
	connectURL string
	messageURL string
	logger     *slog.Logger

	maxPayloadSize int

	requestCancel context.CancelFunc

	messages chan JSONRPCMessage
	closed   chan struct{}
}

// SSEClientOption represents the options for the SSEClient.
type SSEClientOption func(*SSEClient)

type sseServerSession struct {
	id           string
	sess         *sse.Session
	sendMsgs     chan sseServerSessionSendMsg
	receivedMsgs chan JSONRPCMessage
	logger       *slog.Logger

	done           chan struct{}
	sendClosed     chan struct{}
	receivedClosed chan struct{}
}

type sseSessionMessage struct {
	sessID string
	msg    JSONRPCMessage
}

type sseServerSessionSendMsg struct {
	msg  *sse.Message
	errs chan<- error
}

// NewSSEServer creates and initializes a new SSE server that listens for client connections
// at the specified messageURL. The server is immediately operational upon creation with
// initialized internal channels for session and message management. The returned SSEServer
// must be closed using Shutdown when no longer needed.
func NewSSEServer(messageURL string) SSEServer {
	return SSEServer{
		messageURL:       messageURL,
		logger:           slog.Default(),
		sessions:         make(chan sseServerSession, 5),
		removedSessions:  make(chan string),
		receivedMessages: make(chan sseSessionMessage, 100),
		done:             make(chan struct{}),
		closed:           make(chan struct{}),
	}
}

// NewSSEClient creates an SSE client that connects to the specified connectURL. The optional
// httpClient parameter allows custom HTTP client configuration - if nil, the default HTTP
// client is used. The client must call StartSession to begin communication.
func NewSSEClient(connectURL string, httpClient *http.Client, options ...SSEClientOption) *SSEClient {
	cli := httpClient
	if cli == nil {
		cli = http.DefaultClient
	}
	s := &SSEClient{
		connectURL: connectURL,
		httpClient: cli,
		logger:     slog.Default(),
		messages:   make(chan JSONRPCMessage, 10),
		closed:     make(chan struct{}),
	}

	for _, opt := range options {
		opt(s)
	}

	return s
}

// WithSSEClientMaxPayloadSize sets the maximum size of the payload that can be received
// from the server. If the payload size exceeds this limit, the error will be logged and
// the client will be disconnected.
func WithSSEClientMaxPayloadSize(size int) SSEClientOption {
	return func(s *SSEClient) {
		s.maxPayloadSize = size
	}
}

// Sessions returns an iterator over active client sessions. The iterator yields new
// Session instances as clients connect to the server. Use this method to access and
// interact with connected clients through the Session interface.
func (s SSEServer) Sessions() iter.Seq[Session] {
	return func(yield func(Session) bool) {
		defer close(s.closed)

		// Store all active sessions in a map for easy lookup when we receive a new message.
		sessionsMap := make(map[string]sseServerSession)

		for {
			select {
			case <-s.done:
				return
			case sess := <-s.sessions:
				// Received a new session from handler.

				// Process send messages for this session in a separate goroutine
				go sess.processSendMessages()

				// Store the session in the map.
				sessionsMap[sess.id] = sess

				// Forward the session to the caller.
				if !yield(sess) {
					return
				}
			case sessID := <-s.removedSessions:
				// Received a session ID to remove from the sessions map.
				delete(sessionsMap, sessID)
			case msg := <-s.receivedMessages:
				session, ok := sessionsMap[msg.sessID]
				if !ok {
					// Ignore the message if the session is not found, it might already be closed.
					continue
				}

				// Forward the message to the session.
				select {
				case <-s.done:
					return
				case session.receivedMsgs <- msg.msg:
				}
			}
		}
	}
}

// Shutdown gracefully shuts down the SSE server by terminating all active client
// connections and cleaning up internal resources. This method blocks until shutdown
// is complete.
func (s SSEServer) Shutdown(ctx context.Context) error {
	// Signal the server to shutdown.
	close(s.done)

	// Wait for main loop to finish.
	select {
	case <-ctx.Done():
		return fmt.Errorf("failed to close SSE server: %w", ctx.Err())
	case <-s.closed:
	}
	return nil
}

// HandleSSE returns an http.Handler for managing SSE connections over GET requests.
// The handler upgrades HTTP connections to SSE, assigns unique session IDs, and
// provides clients with their message endpoints. The connection remains active until
// either the client disconnects or the server closes.
func (s SSEServer) HandleSSE() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Received the request to establish a new SSE session.
		sess, err := sse.Upgrade(w, r)
		if err != nil {
			nErr := fmt.Errorf("failed to upgrade session: %w", err)
			s.logger.Error("failed to upgrade session", "err", nErr)
			http.Error(w, nErr.Error(), http.StatusInternalServerError)
			return
		}

		sessID := uuid.New().String()

		// Form an url for the client that can be used to communicate with the server session.
		url := fmt.Sprintf("%s?sessionID=%s", s.messageURL, sessID)

		// Use the type "endpoint" to indicate the endpoint URL.
		msg := sse.Message{
			Type: sse.Type("endpoint"),
		}
		msg.AppendData(url)
		if err := sess.Send(&msg); err != nil {
			nErr := fmt.Errorf("failed to write SSE URL: %w", err)
			s.logger.Error("failed to write SSE URL", "err", nErr)
			http.Error(w, nErr.Error(), http.StatusInternalServerError)
			return
		}

		if err := sess.Flush(); err != nil {
			nErr := fmt.Errorf("failed to flush SSE: %w", err)
			s.logger.Error("failed to flush SSE", "err", nErr)
			http.Error(w, nErr.Error(), http.StatusInternalServerError)
			return
		}

		srvSession := sseServerSession{
			id:             sessID,
			sess:           sess,
			logger:         s.logger,
			sendMsgs:       make(chan sseServerSessionSendMsg),
			receivedMsgs:   make(chan JSONRPCMessage),
			done:           make(chan struct{}),
			sendClosed:     make(chan struct{}),
			receivedClosed: make(chan struct{}),
		}

		// Feed the sessions channel that would be consumed in Sessions loop, so it can be fowarded to caller.
		s.sessions <- srvSession

		// Block until the session is closed, so the connection is left open.
		<-srvSession.sendClosed
		<-srvSession.receivedClosed

		// Notify the main loop that this session is closed.
		select {
		case s.removedSessions <- sessID:
		case <-s.done:
		}
	})
}

// HandleMessage returns an http.Handler for processing client messages sent via POST
// requests. The handler expects a sessionID query parameter and a JSON-encoded message
// body. Valid messages are routed to their corresponding Session's message stream,
// accessible through the Sessions iterator.
func (s SSEServer) HandleMessage() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Received a requuest form client to one of our sessions.
		sessID := r.URL.Query().Get("sessionID")
		if sessID == "" {
			nErr := fmt.Errorf("missing sessionID query parameter")
			s.logger.Warn("missing sessionID query parameter", slog.String("err", nErr.Error()))
			http.Error(w, nErr.Error(), http.StatusBadRequest)
			return
		}

		decoder := json.NewDecoder(r.Body)
		var msg JSONRPCMessage

		if err := decoder.Decode(&msg); err != nil {
			nErr := fmt.Errorf("failed to decode message: %w", err)
			s.logger.Warn("failed to decode message", slog.String("err", nErr.Error()))
			http.Error(w, nErr.Error(), http.StatusBadRequest)
			return
		}

		// Feed the receivedMessages channel so the Sessions loop can route it to the correct session.
		select {
		case <-s.done:
			return
		case <-r.Context().Done():
			http.Error(w, "context is cancelled", http.StatusBadRequest)
			s.logger.Warn("context is cancelled while handling message", slog.Any("message", msg))
			return
		case s.receivedMessages <- sseSessionMessage{sessID: sessID, msg: msg}:
		}
	})
}

// StartSession establishes the SSE connection and begins message processing. It sends
// connection status through the ready channel and returns an iterator for received server
// messages. The connection remains active until the context is cancelled or an error occurs.
func (s *SSEClient) StartSession(ctx context.Context) (Session, error) {
	// We cannot use the ctx as the parent context because the caller may cancel the context
	// after calling this function. Since we need a long-lived context, we create a new one, and store
	// the cancel function so we can cancel it when we want to stop the session.
	reqCtx, reqCancel := context.WithCancel(context.Background())
	s.requestCancel = reqCancel

	// But we still need to cancel the request, if the cancellation is happen while the server is not responsing yet,
	// or we still haven't finished the initialization process.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			// If the cancellation come first, then cancel the request.
			reqCancel()
		case <-done:
		}
	}()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, s.connectURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSE server: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Start the sse message listener, and wait until we receive initialization response from the server.
	initErrs := make(chan error)

	go s.listenSSEMessages(resp.Body, initErrs)

	// Wait for initialization response or context cancellation.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-initErrs:
		if err != nil {
			return nil, err
		}
	}

	return s, nil
}

// ID returns the unique identifier for this session.
func (s *SSEClient) ID() string {
	return uuid.New().String()
}

// Send transmits a JSON-encoded message to the server through an HTTP POST request. The
// provided context allows request cancellation. Returns an error if message encoding fails,
// the request cannot be created, or the server responds with a non-200 status code.
func (s *SSEClient) Send(ctx context.Context, msg JSONRPCMessage) error {
	msgBs, err := json.Marshal(msg)
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

// Messages returns an iterator over received messages from the server.
func (s *SSEClient) Messages() iter.Seq[JSONRPCMessage] {
	return func(yield func(JSONRPCMessage) bool) {
		defer close(s.closed)

		for msg := range s.messages {
			if !yield(msg) {
				return
			}
		}
	}
}

// Stop gracefully shuts down the SSE client by closing the SSE connection.
func (s *SSEClient) Stop() {
	// Cancel the request context that made for starting the session to signal the shutdown.
	s.requestCancel()

	// Wait for the main loop to finish.
	<-s.closed
}

func (s *SSEClient) listenSSEMessages(body io.ReadCloser, initErrs chan<- error) {
	defer func() {
		body.Close()
		close(s.messages)
	}()

	// The default value defined in the sse library is 65 KB, set this config if user set a custom value.
	var config *sse.ReadConfig
	if s.maxPayloadSize > 0 {
		config = &sse.ReadConfig{
			MaxEventSize: s.maxPayloadSize,
		}
	}

	// This session would break when the Stop is called, as we cancel the context for this long-lived request.
	for ev, err := range sse.Read(body, config) {
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				s.logger.Error("failed to read SSE message", slog.String("err", err.Error()))
			}
			return
		}

		switch ev.Type {
		case "endpoint":
			// Validate and parse the endpoint URL to ensure secure and correct message routing.
			// This step is critical to prevent potential security vulnerabilities and
			// ensure that messages are sent to the correct destination.
			u, err := url.Parse(ev.Data)
			if err != nil {
				initErrs <- fmt.Errorf("parse endpoint URL: %w", err)
				return
			}
			if u.String() == "" {
				initErrs <- errors.New("empty endpoint URL")
				return
			}
			s.messageURL = u.String()
			close(initErrs)
		case "message":
			if s.messageURL == "" {
				// This should not happen, as we cannot receive message, if we didn't request it to the messageURL,
				// but just in case, we should log it.
				s.logger.Error("received message before endpoint URL")
				continue
			}

			var msg JSONRPCMessage
			if err := json.Unmarshal([]byte(ev.Data), &msg); err != nil {
				s.logger.Error("failed to unmarshal message", slog.String("err", err.Error()))
				continue
			}

			s.messages <- msg

		default:
			s.logger.Error("unhandled event type", "type", ev.Type)
		}
	}
}

func (s sseServerSession) ID() string { return s.id }

func (s sseServerSession) Send(ctx context.Context, msg JSONRPCMessage) error {
	msgBs, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	sseMsg := &sse.Message{
		Type: sse.Type("message"),
	}
	sseMsg.AppendData(string(msgBs))

	errs := make(chan error)

	// Queue the message for sending to avoid race in the sse library
	select {
	case s.sendMsgs <- sseServerSessionSendMsg{sseMsg, errs}:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		s.logger.Warn("session is closed while sending message", slog.String("message", string(msgBs)))
		return fmt.Errorf("session is closed")
	}

	// Wait and return the error if any
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		s.logger.Warn("session is closed while sending message", slog.String("message", string(msgBs)))
		return fmt.Errorf("session is closed")
	}
}

func (s sseServerSession) Messages() iter.Seq[JSONRPCMessage] {
	return func(yield func(JSONRPCMessage) bool) {
		defer close(s.receivedClosed)

		for {
			select {
			case msg := <-s.receivedMsgs:
				if !yield(msg) {
					return
				}
			case <-s.done:
				return
			}
		}
	}
}

func (s sseServerSession) Stop() {
	close(s.done)

	<-s.sendClosed
	<-s.receivedClosed
}

func (s sseServerSession) processSendMessages() {
	defer close(s.sendClosed)

	for {
		select {
		case sm := <-s.sendMsgs:
			// Send and flush the message to the client.
			if err := s.sess.Send(sm.msg); err != nil {
				s.logger.Warn("failed to send message", slog.String("err", err.Error()))

				select {
				case sm.errs <- err:
				default:
				}
				continue
			}
			if err := s.sess.Flush(); err != nil {
				s.logger.Warn("failed to flush message", slog.String("err", err.Error()))

				select {
				case sm.errs <- err:
				default:
				}
				continue
			}

			select {
			case sm.errs <- nil:
			default:
			}
		case <-s.done:
			return
		}
	}
}
