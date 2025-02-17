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
// Instances should be created using NewSSEServer and properly shut down using Close when
// no longer needed.
type SSEServer struct {
	messageURL string
	logger     *slog.Logger

	addSessions     chan sseServerSession
	sessions        chan sseServerSession
	stopSessions    chan string
	removeSessions  chan string
	sessionMessages chan sessionMsg
	done            chan struct{}
	closed          chan struct{}
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

	messages chan JSONRPCMessage
}

type sseServerSession struct {
	id           string
	sess         *sse.Session
	sendMsgs     chan sseServerSessionSendMsg
	receivedMsgs chan JSONRPCMessage

	done           chan struct{}
	closed         chan struct{}
	receivedClosed chan struct{}
}

type sseServerSessionSendMsg struct {
	msg  *sse.Message
	errs chan<- error
}

// NewSSEServer creates and initializes a new SSE server that listens for client connections
// at the specified messageURL. The server is immediately operational upon creation with
// initialized internal channels for session and message management. The returned SSEServer
// must be closed using Close when no longer needed.
func NewSSEServer(messageURL string) SSEServer {
	s := SSEServer{
		messageURL:      messageURL,
		logger:          slog.Default(),
		addSessions:     make(chan sseServerSession),
		sessions:        make(chan sseServerSession, 5),
		stopSessions:    make(chan string),
		removeSessions:  make(chan string),
		sessionMessages: make(chan sessionMsg),
		done:            make(chan struct{}),
		closed:          make(chan struct{}),
	}
	go s.listen()
	return s
}

// NewSSEClient creates an SSE client that connects to the specified connectURL. The optional
// httpClient parameter allows custom HTTP client configuration - if nil, the default HTTP
// client is used. The client must call StartSession to begin communication.
func NewSSEClient(connectURL string, httpClient *http.Client) *SSEClient {
	cli := httpClient
	if cli == nil {
		cli = http.DefaultClient
	}
	return &SSEClient{
		connectURL: connectURL,
		httpClient: cli,
		logger:     slog.Default(),
		messages:   make(chan JSONRPCMessage),
	}
}

// Sessions returns an iterator over active client sessions. The iterator yields new
// Session instances as clients connect to the server. Use this method to access and
// interact with connected clients through the Session interface.
func (s SSEServer) Sessions() iter.Seq[Session] {
	return func(yield func(Session) bool) {
		for sess := range s.sessions {
			if !yield(sess) {
				return
			}
		}
	}
}

// StopSession stops the session with the given ID.
func (s SSEServer) StopSession(sessID string) {
	select {
	case s.stopSessions <- sessID:
	case <-s.done:
	}
}

// HandleSSE returns an http.Handler for managing SSE connections over GET requests.
// The handler upgrades HTTP connections to SSE, assigns unique session IDs, and
// provides clients with their message endpoints. The connection remains active until
// either the client disconnects or the server closes.
func (s SSEServer) HandleSSE() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := sse.Upgrade(w, r)
		if err != nil {
			nErr := fmt.Errorf("failed to upgrade session: %w", err)
			s.logger.Error("failed to upgrade session", "err", nErr)
			http.Error(w, nErr.Error(), http.StatusInternalServerError)
			return
		}

		sessID := uuid.New().String()

		url := fmt.Sprintf("%s?sessionID=%s", s.messageURL, sessID)
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

		s.addSessions <- sseServerSession{
			id:             sessID,
			sess:           sess,
			sendMsgs:       make(chan sseServerSessionSendMsg),
			receivedMsgs:   make(chan JSONRPCMessage, 5),
			done:           make(chan struct{}),
			closed:         make(chan struct{}),
			receivedClosed: make(chan struct{}),
		}

		<-r.Context().Done()

		select {
		case s.removeSessions <- sessID:
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
		sessID := r.URL.Query().Get("sessionID")
		if sessID == "" {
			nErr := fmt.Errorf("missing sessionID query parameter")
			s.logger.Error("missing sessionID query parameter", "err", nErr)
			http.Error(w, nErr.Error(), http.StatusBadRequest)
			return
		}

		decoder := json.NewDecoder(r.Body)
		var msg JSONRPCMessage

		if err := decoder.Decode(&msg); err != nil {
			nErr := fmt.Errorf("failed to decode message: %w", err)
			s.logger.Error("failed to decode message", "err", nErr)
			http.Error(w, nErr.Error(), http.StatusBadRequest)
			return
		}

		s.sessionMessages <- sessionMsg{sessionID: sessID, msg: msg}
	})
}

// Close gracefully shuts down the SSE server by terminating all active client
// connections and cleaning up internal resources. This method blocks until shutdown
// is complete.
func (s SSEServer) Close() {
	close(s.done)
	<-s.closed
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

// StartSession establishes the SSE connection and begins message processing. It sends
// connection status through the ready channel and returns an iterator for received server
// messages. The connection remains active until the context is cancelled or an error occurs.
func (s *SSEClient) StartSession(ctx context.Context, ready chan<- error) (iter.Seq[JSONRPCMessage], error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.connectURL, nil)
	if err != nil {
		close(ready)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		close(ready)
		return nil, fmt.Errorf("failed to connect to SSE server: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		close(ready)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	go s.listenSSEMessages(resp.Body, ready)

	return s.listenMessages(), nil
}

func (s SSEServer) listen() {
	defer close(s.closed)

	// Track all active sessions in a map for O(1) lookup and removal
	sessions := make(map[string]sseServerSession)

	for {
		select {
		case <-s.done:
			return
		case sess := <-s.addSessions:
			// We start the session handler goroutine before adding it to the map
			// to ensure message processing begins immediately
			go sess.start()
			sessions[sess.id] = sess
			s.sessions <- sess
		case sessID := <-s.stopSessions:
			sess, ok := sessions[sessID]
			if !ok {
				continue
			}
			sess.stop()
			delete(sessions, sessID)
		case sessID := <-s.removeSessions:
			delete(sessions, sessID)
		case sessMsg := <-s.sessionMessages:
			sess, ok := sessions[sessMsg.sessionID]
			if !ok {
				continue
			}
			sess.receivedMsgs <- sessMsg.msg
		}
	}
}

func (s *SSEClient) listenSSEMessages(body io.ReadCloser, ready chan<- error) {
	defer body.Close()

	for ev, err := range sse.Read(body, nil) {
		if err != nil {
			return
		}

		switch ev.Type {
		case "endpoint":
			// We must receive and validate the endpoint URL before processing any messages
			// to ensure proper message routing
			u, err := url.Parse(ev.Data)
			if err != nil {
				ready <- fmt.Errorf("parse endpoint URL: %w", err)
				return
			}
			if u.String() == "" {
				ready <- errors.New("empty endpoint URL")
				return
			}
			s.messageURL = u.String()
			close(ready)
		case "message":
			// We enforce the requirement that an endpoint URL must be received
			// before processing any messages
			if s.messageURL == "" {
				s.logger.Error("received message before endpoint URL")
				continue
			}

			var msg JSONRPCMessage
			if err := json.Unmarshal([]byte(ev.Data), &msg); err != nil {
				s.logger.Error("failed to unmarshal message", "err", err)
				continue
			}

			s.messages <- msg
		default:
			s.logger.Error("unhandled event type", "type", ev.Type)
		}
	}
}

func (s *SSEClient) listenMessages() iter.Seq[JSONRPCMessage] {
	return func(yield func(JSONRPCMessage) bool) {
		for msg := range s.messages {
			if !yield(msg) {
				return
			}
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
	sm := sseServerSessionSendMsg{
		msg:  sseMsg,
		errs: errs,
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return nil
	case s.sendMsgs <- sm:
	}

	return <-errs
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

func (s sseServerSession) start() {
	defer close(s.closed)

	for {
		select {
		case sm := <-s.sendMsgs:
			if err := s.sess.Send(sm.msg); err != nil {
				sm.errs <- fmt.Errorf("failed to send message: %w", err)
				continue
			}
			if err := s.sess.Flush(); err != nil {
				sm.errs <- fmt.Errorf("failed to flush message: %w", err)
				continue
			}
			sm.errs <- nil
		case <-s.done:
			return
		}
	}
}

func (s sseServerSession) stop() {
	close(s.done)
	close(s.receivedMsgs)

	<-s.closed
	<-s.receivedClosed
}
