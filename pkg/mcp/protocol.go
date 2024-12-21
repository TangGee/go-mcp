package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// JSONRPCMessage represents a JSON-RPC 2.0 message used for communication in the MCP protocol.
// It can represent either a request, response, or notification depending on which fields are populated:
//   - Request: JSONRPC, ID, Method, and Params are set
//   - Response: JSONRPC, ID, and either Result or Error are set
//   - Notification: JSONRPC and Method are set (no ID)
type JSONRPCMessage struct {
	// JSONRPC must always be "2.0" per the JSON-RPC specification
	JSONRPC string `json:"jsonrpc"`
	// ID uniquely identifies request-response pairs and must be a string or number
	ID MustString `json:"id,omitempty"`
	// Method contains the RPC method name for requests and notifications
	Method string `json:"method,omitempty"`
	// Params contains the parameters for the method call as a raw JSON message
	Params json.RawMessage `json:"params,omitempty"`
	// Result contains the successful response data as a raw JSON message
	Result json.RawMessage `json:"result,omitempty"`
	// Error contains error details if the request failed
	Error *jsonRPCError `json:"error,omitempty"`
}

// Info contains metadata about a server or client instance including its name and version.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MustString is a type that enforces string representation for fields that can be either string or integer
// in the protocol specification, such as request IDs and progress tokens. It handles automatic conversion
// during JSON marshaling/unmarshaling.
type MustString string

// CompletionCompleteParams contains parameters for requesting completion suggestions.
// It includes a reference to what is being completed (e.g. a prompt or resource template)
// and the specific argument that needs completion suggestions.
type CompletionCompleteParams struct {
	// Ref identifies what is being completed (e.g. prompt, resource template)
	Ref CompletionCompleteRef `json:"ref"`
	// Argument specifies which argument needs completion suggestions
	Argument CompletionArgument `json:"argument"`
}

// CompletionCompleteRef identifies what is being completed in a completion request.
// Type must be one of:
//   - "ref/prompt": Completing a prompt argument, Name field must be set to prompt name
//   - "ref/resource": Completing a resource template argument, URI field must be set to template URI
type CompletionCompleteRef struct {
	// Type specifies what kind of completion is being requested.
	// Must be either "ref/prompt" or "ref/resource".
	Type string `json:"type"`
	// Name contains the prompt name when Type is "ref/prompt".
	Name string `json:"name,omitempty"`
	// URI contains the resource template URI when Type is "ref/resource".
	URI string `json:"uri,omitempty"`
}

// CompletionArgument defines the structure for arguments passed in completion requests,
// containing the argument name and its corresponding value.
type CompletionArgument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CompletionResult contains the response data for a completion request, including
// possible completion values and whether more completions are available.
type CompletionResult struct {
	Completion struct {
		Values  []string `json:"values"`
		HasMore bool     `json:"hasMore"`
	} `json:"completion"`
}

// ParamsMeta contains optional metadata that can be included with request parameters.
// It is used to enable features like progress tracking for long-running operations.
type ParamsMeta struct {
	// ProgressToken uniquely identifies an operation for progress tracking.
	// When provided, the server can emit progress updates via ProgressReporter.
	ProgressToken MustString `json:"progressToken"`
}

type initializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Info               `json:"clientInfo"`
}

type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Info               `json:"serverInfo"`
}

type notificationsCancelledParams struct {
	RequestID string `json:"requestId"`
	Reason    string `json:"reason"`
}

type notificationsResourcesUpdatedParams struct {
	URI string `json:"uri"`
}

type jsonRPCError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

const (
	// CompletionRefPrompt is used in CompletionCompleteRef.Type for prompt argument completion.
	CompletionRefPrompt = "ref/prompt"
	// CompletionRefResource is used in CompletionCompleteRef.Type for resource template argument completion.
	CompletionRefResource = "ref/resource"

	// JSONRPCVersion specifies the JSON-RPC protocol version used for communication.
	JSONRPCVersion = "2.0"

	// MethodPromptsList is the method name for retrieving a list of available prompts.
	MethodPromptsList = "prompts/list"
	// MethodPromptsGet is the method name for retrieving a specific prompt by identifier.
	MethodPromptsGet = "prompts/get"

	// MethodResourcesList is the method name for listing available resources.
	MethodResourcesList = "resources/list"
	// MethodResourcesRead is the method name for reading the content of a specific resource.
	MethodResourcesRead = "resources/read"
	// MethodResourcesTemplatesList is the method name for listing available resource templates.
	MethodResourcesTemplatesList = "resources/templates/list"
	// MethodResourcesSubscribe is the method name for subscribing to resource updates.
	MethodResourcesSubscribe = "resources/subscribe"

	// MethodToolsList is the method name for retrieving a list of available tools.
	MethodToolsList = "tools/list"
	// MethodToolsCall is the method name for invoking a specific tool.
	MethodToolsCall = "tools/call"

	// MethodRootsList is the method name for retrieving a list of root resources.
	MethodRootsList = "roots/list"
	// MethodSamplingCreateMessage is the method name for creating a new sampling message.
	MethodSamplingCreateMessage = "sampling/createMessage"

	// MethodCompletionComplete is the method name for requesting completion suggestions.
	MethodCompletionComplete = "completion/complete"

	protocolVersion = "2024-11-05"

	errMsgInvalidJSON                    = "Invalid json"
	errMsgUnsupportedProtocolVersion     = "Unsupported protocol version"
	errMsgInsufficientClientCapabilities = "Insufficient client capabilities"
	errMsgInternalError                  = "Internal error"
	errMsgWriteTimeout                   = "Write timeout"
	errMsgReadTimeout                    = "Read timeout"

	methodPing       = "ping"
	methodInitialize = "initialize"

	methodNotificationsInitialized          = "notifications/initialized"
	methodNotificationsCancelled            = "notifications/cancelled"
	methodNotificationsPromptsListChanged   = "notifications/prompts/list_changed"
	methodNotificationsResourcesListChanged = "notifications/resources/list_changed"
	methodNotificationsResourcesUpdated     = "notifications/resources/updated"
	methodNotificationsToolsListChanged     = "notifications/tools/list_changed"
	methodNotificationsProgress             = "notifications/progress"
	methodNotificationsMessage              = "notifications/message"

	methodNotificationsRootsListChanged = "notifications/roots/list_changed"

	userCancelledReason = "User requested cancellation"

	jsonRPCParseErrorCode     = -32700
	jsonRPCInvalidRequestCode = -32600
	jsonRPCMethodNotFoundCode = -32601
	jsonRPCInvalidParamsCode  = -32602
	jsonRPCInternalErrorCode  = -32603
)

func readMessage(r io.Reader) (JSONRPCMessage, error) {
	decoder := json.NewDecoder(r)
	var msg JSONRPCMessage

	if err := decoder.Decode(&msg); err != nil {
		return JSONRPCMessage{}, fmt.Errorf("failed to decode message: %w", err)
	}

	if msg.JSONRPC != JSONRPCVersion {
		return JSONRPCMessage{}, fmt.Errorf("invalid jsonrpc version: %s", msg.JSONRPC)
	}

	return msg, nil
}

func writeError(ctx context.Context, w io.Writer, id MustString, err jsonRPCError) error {
	response := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   &err,
	}

	return writeMessage(ctx, w, response)
}

func writeResult(ctx context.Context, w io.Writer, id MustString, result any) error {
	resBs, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  resBs,
	}

	return writeMessage(ctx, w, msg)
}

func writeParams(ctx context.Context, w io.Writer, id MustString, method string, params any) error {
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Method:  method,
		Params:  paramsBs,
	}

	return writeMessage(ctx, w, msg)
}

func writeNotifications(ctx context.Context, w io.Writer, method string, params any) error {
	paramsBs, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  paramsBs,
	}

	return writeMessage(ctx, w, msg)
}

func writeMessage(ctx context.Context, w io.Writer, msg JSONRPCMessage) error {
	encoder := json.NewEncoder(w)
	errChan := make(chan error)

	go func() {
		err := encoder.Encode(&msg)
		if err != nil {
			errChan <- fmt.Errorf("failed to write message: %w", err)
			return
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// UnmarshalJSON implements json.Unmarshaler to convert JSON data into MustString,
// handling both string and numeric input formats.
func (m *MustString) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	switch v := v.(type) {
	case string:
		*m = MustString(v)
	case float64:
		*m = MustString(fmt.Sprintf("%d", int(v)))
	case int:
		*m = MustString(fmt.Sprintf("%d", v))
	default:
		return fmt.Errorf("invalid type: %T", v)
	}

	return nil
}

// MarshalJSON implements json.Marshaler to convert MustString into its JSON representation,
// always encoding as a string value.
func (m MustString) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(m))
}

func (j jsonRPCError) Error() string {
	return fmt.Sprintf("request error, code: %d, message: %s, data %v", j.Code, j.Message, j.Data)
}
