package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// Info contains metadata about a server or client instance including its name and version.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MustString is a type that enforces string representation for fields that can be either string or integer
// in the protocol specification, such as request IDs and progress tokens. It handles automatic conversion
// during JSON marshaling/unmarshaling.
type MustString string

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

type completionCompleteParams struct {
	Ref struct {
		Type string `json:"type"`
		Name string `json:"name,omitempty"`
		URI  string `json:"uri,omitempty"`
	} `json:"ref"`
	Argument CompletionArgument `json:"argument"`
}

type paramsMeta struct {
	ProgressToken MustString `json:"progressToken"`
}

type jsonRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      MustString      `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

const (
	protocolVersion = "2024-11-05"
	jsonRPCVersion  = "2.0"

	errMsgInvalidJSON                    = "Invalid json"
	errMsgUnsupportedProtocolVersion     = "Unsupported protocol version"
	errMsgInsufficientClientCapabilities = "Insufficient client capabilities"
	errMsgInternalError                  = "Internal error"
	errMsgWriteTimeout                   = "Write timeout"
	errMsgReadTimeout                    = "Read timeout"

	methodPing       = "ping"
	methodInitialize = "initialize"

	methodPromptsList = "prompts/list"
	methodPromptsGet  = "prompts/get"

	methodResourcesList          = "resources/list"
	methodResourcesRead          = "resources/read"
	methodResourcesTemplatesList = "resources/templates/list"
	methodResourcesSubscribe     = "resources/subscribe"

	methodToolsList = "tools/list"
	methodToolsCall = "tools/call"

	methodRootsList             = "roots/list"
	methodSamplingCreateMessage = "sampling/createMessage"

	methodCompletionComplete = "completion/complete"

	methodNotificationsInitialized          = "notifications/initialized"
	methodNotificationsCancelled            = "notifications/cancelled"
	methodNotificationsPromptsListChanged   = "notifications/prompts/list_changed"
	methodNotificationsResourcesListChanged = "notifications/resources/list_changed"
	methodNotificationsResourcesSubscribe   = "notifications/resources/subscribe"
	methodNotificationsToolsListChanged     = "notifications/tools/list_changed"
	methodNotificationsProgress             = "notifications/progress"
	methodNotificationsMessage              = "notifications/message"

	methodNotificationsRootsListChanged = "notifications/roots/list_changed"

	jsonRPCParseErrorCode     = -32700
	jsonRPCInvalidRequestCode = -32600
	jsonRPCMethodNotFoundCode = -32601
	jsonRPCInvalidParamsCode  = -32602
	jsonRPCInternalErrorCode  = -32603
)

func readMessage(r io.Reader) (jsonRPCMessage, error) {
	decoder := json.NewDecoder(r)
	var msg jsonRPCMessage

	if err := decoder.Decode(&msg); err != nil {
		return jsonRPCMessage{}, fmt.Errorf("failed to decode message: %w", err)
	}

	if msg.JSONRPC != jsonRPCVersion {
		return jsonRPCMessage{}, fmt.Errorf("invalid jsonrpc version: %s", msg.JSONRPC)
	}

	return msg, nil
}

func writeError(ctx context.Context, w io.Writer, id MustString, err jsonRPCError) error {
	response := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
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

	msg := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
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

	msg := jsonRPCMessage{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Method:  method,
		Params:  paramsBs,
	}

	return writeMessage(ctx, w, msg)
}

func writeMessage(ctx context.Context, w io.Writer, msg jsonRPCMessage) error {
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
