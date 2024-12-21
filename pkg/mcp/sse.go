package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
)

// SSEServer implements a Server-Sent Events (SSE) server that wraps an MCP server implementation.
// It manages SSE connections and handles bidirectional communication between clients and the underlying MCP server.
// The server maintains a mapping of session IDs to writers for sending events to connected clients.
type SSEServer struct {
	srv server

	writers *sync.Map // map[sessionID]io.Writer
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
		srv:     s,
		writers: new(sync.Map),
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
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		sessID := s.srv.startSession(r.Context(), w)
		s.writers.Store(sessID, w)

		url := fmt.Sprintf("%s?sessionID=%s", messageBaseURL, sessID)
		_, err := w.Write([]byte(url))
		if err != nil {
			log.Printf("failed to write SSE URL: %v", err)
		}

		// Keep the connection open for new messages
		<-r.Context().Done()
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

		sw, ok := s.writers.Load(sessID)
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		if err := s.srv.handleMsg(r.Body, sessID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		f, ok := sw.(http.Flusher)
		if ok {
			f.Flush()
		}
	})
}

// Stop gracefully shuts down the SSE server by stopping the underlying MCP server.
// This will close all active sessions and stop processing events.
func (s SSEServer) Stop() {
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

	// The first message from the server contains the message URL
	scanner := bufio.NewScanner(resp.Body)
	if !scanner.Scan() {
		resp.Body.Close()
		return "", fmt.Errorf("failed to read message URL")
	}
	msgURL := scanner.Text()

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
	}

	s.cli.startSession(ctx, sw, sessID)

	go func() {
		defer resp.Body.Close()
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			input := scanner.Text()

			var res JSONRPCMessage
			if err := json.Unmarshal([]byte(input), &res); err != nil {
				log.Printf("failed to unmarshal SSE event: %v", err)
				continue
			}

			if err := s.cli.handleMsg(bytes.NewReader([]byte(input)), sessID); err != nil {
				log.Printf("failed to handle SSE event: %v", err)
				continue
			}
		}

		if scanner.Err() != nil {
			log.Printf("failed to read SSE events: %v", scanner.Err())
		}
	}()

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
func (s SSEClient) GetPrompt(ctx context.Context, sessionID string, params PromptsGetParams) (Prompt, error) {
	ctx = ctxWithSessionID(ctx, sessionID)
	return s.cli.getPrompt(ctx, params.Name, params.Arguments, params.Meta.ProgressToken)
}

// GetResources retrieves a paginated list of available resources from the server.
//
// The ctx parameter controls the lifecycle of the request and can be used to cancel it.
// The sessionID parameter must be a valid session ID obtained from Connect().
// The params parameter contains optional pagination cursor and progress tracking metadata.
//
// Returns a ResourceList containing the available resources and pagination information.
// Returns an error if the request fails or the session ID is invalid.
func (s SSEClient) GetResources(
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

func (s sseWritter) Write(p []byte) (int, error) {
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
