package mcp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// SSEServer implements a Server-Sent Events (SSE) server that wraps an MCP server implementation.
// It manages SSE connections and handles bidirectional communication between clients and the underlying MCP server.
// The server maintains a mapping of session IDs to writers for sending events to connected clients.
type SSEServer struct {
	srv server

	writers   *sync.Map // map[sessionID]io.Writer
	closeChan chan struct{}

	flushLock *sync.Mutex
}

// SSEClient implements a Server-Sent Events (SSE) client that connects to an MCP SSE server.
// It handles the connection and provides methods to make requests to the server.
type SSEClient struct {
	cli        client
	baseURL    string
	httpClient *http.Client
}

type sseWritter struct {
	httpClient *http.Client
	url        string
	writeLock  *sync.Mutex
}

// NewSSEServer creates a new SSE server instance wrapping the provided MCP server implementation.
// It initializes the underlying server with the given options and starts listening for events.
//
// The server parameter specifies the MCP Server implementation to wrap.
// The option parameter provides optional ServerOptions to configure the underlying server behavior.
//
// Returns an initialized SSEServer ready to handle SSE connections.
func NewSSEServer(server Server, option ...ServerOption) SSEServer {
	s := newServer(server, option...)
	s.start()
	return SSEServer{
		srv:       s,
		writers:   new(sync.Map),
		closeChan: make(chan struct{}),
		flushLock: new(sync.Mutex),
	}
}

// NewSSEClient creates a new Server-Sent Events (SSE) client instance that connects to an MCP SSE server.
// It initializes the underlying client with the provided options and configures the HTTP client for SSE communication.
//
// The client parameter specifies the MCP Client implementation to wrap.
// The baseURL parameter defines the server endpoint URL to connect to.
// The httpClient parameter allows customizing the HTTP client used for SSE connections.
// The option parameter provides optional ClientOptions to configure the underlying client behavior.
//
// Returns an initialized SSEClient ready to establish SSE connections.
func NewSSEClient(client Client, baseURL string, httpClient *http.Client, option ...ClientOption) SSEClient {
	return SSEClient{
		cli:        newClient(client, option...),
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// HandleSSE returns an http.Handler that manages SSE connections from clients.
// It sets up appropriate SSE headers, creates a new session, and keeps the connection alive
// for streaming events. The handler sends the initial message URL to the client for subsequent
// message submissions.
//
// The messageBaseURL parameter defines the base URL where clients should submit messages for this SSE connection.
//
// The returned handler maintains the SSE connection until the client disconnects or the context is cancelled.
func (s SSEServer) HandleSSE(messageBaseURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// Disable chunked encoding to avoid issues with SSE
		w.Header().Set("Transfer-Encoding", "identity")

		fmFunc := func(msg []byte) []byte {
			return []byte(fmt.Sprintf("message: %s\n\n", msg))
		}

		sessID := s.srv.startSession(r.Context(), w, fmFunc, s.flush)
		s.writers.Store(sessID, w)

		url := fmt.Sprintf("%s?sessionID=%s", messageBaseURL, sessID)
		_, err := fmt.Fprintf(w, "endpoint: %s\n\n", url)
		if err != nil {
			log.Printf("failed to write SSE URL: %v", err)
			return
		}

		// Always flush after writing
		s.flush(sessID)

		// Keep the connection open for new messages
		select {
		case <-r.Context().Done():
		case <-s.closeChan:
		}
		// Session would be removed by server when r.Context is done.
	})
}

// HandleMessage returns an http.Handler that processes incoming messages from SSE clients.
// It expects a sessionID query parameter to identify the associated SSE connection.
// Messages are forwarded to the underlying MCP server for processing.
//
// The handler validates:
//   - Presence of sessionID parameter
//   - Existence of the specified session
//   - Message handling by the underlying server
//
// After successful message handling, it flushes the response to ensure immediate delivery.
func (s SSEServer) HandleMessage() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessID := r.URL.Query().Get("sessionID")
		if sessID == "" {
			http.Error(w, "missing sessionID query parameter", http.StatusBadRequest)
			return
		}

		if err := s.srv.handleMsg(r.Body, sessID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	})
}

// RequestListRoot retrieves a list of available roots from the client.
// The context controls the request lifecycle and can be used for cancellation.
func (s SSEServer) RequestListRoot(ctx context.Context) (RootList, error) {
	return s.srv.listRoots(ctx)
}

// RequestSampling sends a sampling request to generate an AI model response based on the provided parameters.
// The context controls the request lifecycle and can be used for cancellation. The params define the sampling
// configuration including conversation history, model preferences, system prompts, and token limits. It returns
// a SamplingResult containing the generated response and model information, or an error if the sampling fails
// or the session is not found.
func (s SSEServer) RequestSampling(ctx context.Context, params SamplingParams) (SamplingResult, error) {
	return s.srv.requestSampling(ctx, params)
}

// Stop gracefully shuts down the SSE server by stopping the underlying MCP server.
// This will close all active sessions and stop processing events.
func (s SSEServer) Stop() {
	close(s.closeChan)
	s.srv.stop()
}

// Connect establishes a Server-Sent Events (SSE) connection to the configured server endpoint.
// It initiates the connection, handles the initial handshake, and starts listening for server events.
//
// The ctx parameter controls the lifecycle of the connection and can be used to cancel it.
//
// Returns a session ID string that uniquely identifies this connection and must be used for subsequent
// API calls. Returns an error if the connection cannot be established, the server returns an unexpected
// status code, or the initial handshake fails.
func (s SSEClient) Connect(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	//nolint:bodyclose // The body would be closed when goroutine below is done
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
	var msgURL string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "endpoint: ") {
			msgURL = strings.TrimSpace(strings.TrimPrefix(line, "endpoint: "))
			break
		}
	}
	if msgURL == "" {
		resp.Body.Close()
		return "", fmt.Errorf("failed to read message URL from SSE event")
	}

	// Parse the session ID from the message URL
	parsedURL, err := url.Parse(msgURL)
	if err != nil {
		resp.Body.Close()
		return "", fmt.Errorf("invalid message URL: %w", err)
	}

	sessID := parsedURL.Query().Get("sessionID")
	if sessID == "" {
		resp.Body.Close()
		return "", fmt.Errorf("no session ID in message URL")
	}

	sw := sseWritter{
		httpClient: s.httpClient,
		url:        msgURL,
		writeLock:  &sync.Mutex{},
	}

	s.cli.startSession(ctx, sw, sessID)

	initReadyChan := make(chan struct{})
	errChan := make(chan error)

	go s.initialize(ctx, sessID, initReadyChan, errChan)

	go func() {
		defer resp.Body.Close()

		newScanner := bufio.NewScanner(resp.Body)
		for newScanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := newScanner.Text()
			if line == "" {
				// Skip empty lines
				continue
			}
			var input string
			if strings.HasPrefix(line, "message: ") {
				input = strings.TrimSpace(strings.TrimPrefix(line, "message: "))
			}

			if err := s.cli.handleMsg(bytes.NewReader([]byte(input)), sessID); err != nil {
				log.Printf("failed to handle SSE event %s: %v", line, err)
				continue
			}
		}

		if newScanner.Err() != nil {
			if !errors.Is(newScanner.Err(), context.Canceled) {
				log.Printf("failed to read SSE events: %v", newScanner.Err())
			}
		}
	}()

	select {
	case err := <-errChan:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	case <-initReadyChan:
	}

	return sessID, nil
}

// ListPrompts retrieves a paginated list of available prompts from the server.
//
// The ctx parameter controls the lifecycle of the request and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains optional pagination cursor and progress tracking metadata.
//
// Returns a PromptList containing the available prompts and pagination information.
// Returns an error if the request fails or the session ID is invalid.
func (s SSEClient) ListPrompts(ctx context.Context, sessionID string, params PromptsListParams) (PromptList, error) {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.listPrompts(ctx, params.Cursor, params.Meta.ProgressToken)
}

// GetPrompt retrieves a specific prompt template by name with the provided arguments.
//
// The ctx parameter controls the lifecycle of the request and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains the prompt name, arguments, and optional progress tracking metadata.
//
// Returns the requested Prompt with its full template and metadata.
// Returns an error if the prompt doesn't exist, the arguments are invalid, or the session ID is invalid.
func (s SSEClient) GetPrompt(ctx context.Context, sessionID string, params PromptsGetParams) (PromptResult, error) {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.getPrompt(ctx, params.Name, params.Arguments, params.Meta.ProgressToken)
}

// CompletesPrompt requests completion suggestions for a prompt argument from the server.
//
// The ctx parameter controls the lifecycle of the request and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains the prompt name and argument to get completions for.
//
// Returns a CompletionResult containing possible completion values and pagination info.
// Returns an error if the session ID is invalid.
func (s SSEClient) CompletesPrompt(
	ctx context.Context,
	sessionID string,
	params CompletionCompleteParams,
) (CompletionResult, error) {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.completesPrompt(ctx, params.Ref.Name, params.Argument)
}

// ListResources retrieves a paginated list of available resources from the server.
//
// The ctx parameter controls the lifecycle of the request and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains optional pagination cursor and progress tracking metadata.
//
// Returns a ResourceList containing the available resources and pagination information.
// Returns an error if the request fails or the session ID is invalid.
func (s SSEClient) ListResources(
	ctx context.Context,
	sessionID string,
	params ResourcesListParams,
) (ResourceList, error) {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.listResources(ctx, params.Cursor, params.Meta.ProgressToken)
}

// ReadResource retrieves the content and metadata of a specific resource by its URI.
//
// The ctx parameter controls the lifecycle of the request and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains the resource URI and optional progress tracking metadata.
//
// Returns the requested Resource with its content and metadata.
// Returns an error if the resource doesn't exist or the session ID is invalid.
func (s SSEClient) ReadResource(ctx context.Context, sessionID string, params ResourcesReadParams) (Resource, error) {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.readResource(ctx, params.URI, params.Meta.ProgressToken)
}

// ListResourceTemplates retrieves all available resource templates from the server.
//
// The ctx parameter controls the lifecycle of the request and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains optional progress tracking metadata.
//
// Returns a slice of ResourceTemplate objects describing the available templates.
// Returns an error if the request fails or the session ID is invalid.
func (s SSEClient) ListResourceTemplates(
	ctx context.Context,
	sessionID string,
	params ResourcesTemplatesListParams,
) ([]ResourceTemplate, error) {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.listResourceTemplates(ctx, params.Meta.ProgressToken)
}

// SubscribeResource registers interest in receiving updates for a specific resource.
// When the resource changes, the server will send notifications via the SSE connection.
//
// The ctx parameter controls the lifecycle of the subscription and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains the URI of the resource to subscribe to.
//
// Returns an error if the subscription fails, the resource doesn't exist, or the session ID is invalid.
func (s SSEClient) SubscribeResource(ctx context.Context, sessionID string, params ResourcesSubscribeParams) error {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.subscribeResource(ctx, params.URI)
}

// UnsubscribeResource cancels an existing subscription for updates to a specific resource.
// This removes the registration previously created by SubscribeResource, stopping any further
// notifications for that resource via the SSE connection.
//
// The ctx parameter controls the lifecycle of the unsubscribe request and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains the URI of the resource to unsubscribe from.
//
// Returns an error if the unsubscription fails, the resource doesn't exist, or the session ID is invalid.
func (s SSEClient) UnsubscribeResource(ctx context.Context, sessionID string, params ResourcesSubscribeParams) error {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.unsubscribeResource(ctx, params.URI)
}

// CompletesResourceTemplate provides completion suggestions for a resource template argument.
//
// The ctx parameter controls the lifecycle of the request and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains the template URI and argument to get completions for.
//
// Returns a CompletionResult containing possible completion values and pagination info.
// Returns an error if the session ID is invalid.
func (s SSEClient) CompletesResourceTemplate(
	ctx context.Context,
	sessionID string,
	params CompletionCompleteParams,
) (CompletionResult, error) {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.completesResourceTemplate(ctx, params.Ref.Name, params.Argument)
}

// ListTools retrieves a paginated list of available tools from the server.
//
// The ctx parameter controls the lifecycle of the request and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains optional pagination cursor and progress tracking metadata.
//
// Returns a ToolList containing the available tools and pagination information.
// Returns an error if the request fails or the session ID is invalid.
func (s SSEClient) ListTools(ctx context.Context, sessionID string, params ToolsListParams) (ToolList, error) {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.listTools(ctx, params.Cursor, params.Meta.ProgressToken)
}

// CallTool executes a specific tool with the provided arguments.
//
// The ctx parameter controls the lifecycle of the tool execution and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains the tool name, arguments, and optional progress tracking metadata.
//
// Returns a ToolResult containing the execution outcome and any output data.
// Returns an error if the tool doesn't exist, the arguments are invalid, or the session ID is invalid.
func (s SSEClient) CallTool(ctx context.Context, sessionID string, params ToolsCallParams) (ToolResult, error) {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.callTool(ctx, params.Name, params.Arguments, params.Meta.ProgressToken)
}

// SetLogLevel sets the minimum severity level for emitted log messages.
func (s SSEClient) SetLogLevel(level LogLevel) error {
	return s.cli.setLogLevel(level)
}

func (s SSEServer) flush(sessID string) {
	s.flushLock.Lock()
	defer s.flushLock.Unlock()

	// Flush the writer after each message
	sw, ok := s.writers.Load(sessID)
	if !ok {
		return
	}
	f, ok := sw.(http.Flusher)
	if ok {
		f.Flush()
	}
}

func (s SSEClient) initialize(ctx context.Context, sessID string, readyChan chan<- struct{}, errChan chan<- error) {
	defer close(readyChan)
	ctx = ctxWithSessionID(ctx, sessID)
	if err := s.cli.initialize(ctx); err != nil {
		errChan <- fmt.Errorf("failed to initialize session: %w", err)
		return
	}
}

func (s sseWritter) Write(p []byte) (int, error) {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.url, bytes.NewReader(p))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return len(p), nil
}
