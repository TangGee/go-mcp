package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

type serverSession struct {
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	writter io.Writer

	clientCapabilities ClientCapabilities
	clientInfo         Info

	writeTimeout time.Duration
	readTimeout  time.Duration
	pingInterval time.Duration

	clientRequests      sync.Map // map[requestID]clientRequest
	serverRequests      sync.Map // map[requestID]chan jsonRPCMessage
	subscribedResources sync.Map // map[uri]struct{}

	promptsListChan        chan struct{}
	resourcesListChan      chan struct{}
	resourcesSubscribeChan chan string
	toolsListChan          chan struct{}
	logChan                chan LogParams
	progressChan           chan ProgressParams
	stopChan               chan<- string

	initLock    sync.RWMutex
	initialized bool
}

type clientRequest struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type sessionIDKeyType string

const sessionIDKey sessionIDKeyType = "sessionID"

func sessionIDFromContext(ctx context.Context) string {
	v := ctx.Value(sessionIDKey)
	if v == nil {
		return ""
	}
	id, _ := v.(string)
	return id
}

func ctxWithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey, id)
}

func (s *serverSession) listen() {
	pingTicker := time.NewTicker(s.pingInterval)

	for {
		select {
		case <-s.ctx.Done():
			s.stopChan <- s.id
			return
		case <-s.promptsListChan:
			_ = writeResult(s.ctx, s.writter, methodNotificationsPromptsListChanged, nil)
		case <-s.resourcesListChan:
			_ = writeResult(s.ctx, s.writter, methodNotificationsResourcesListChanged, nil)
		case uri := <-s.resourcesSubscribeChan:
			_, ok := s.subscribedResources.Load(uri)
			if !ok {
				continue
			}
			_ = writeResult(s.ctx, s.writter, methodNotificationsResourcesSubscribe, uri)
		case <-s.toolsListChan:
			_ = writeResult(s.ctx, s.writter, methodNotificationsToolsListChanged, nil)
		case params := <-s.logChan:
			_ = writeResult(s.ctx, s.writter, methodNotificationsMessage, params)
		case params := <-s.progressChan:
			_ = writeResult(s.ctx, s.writter, methodNotificationsProgress, params)
		case <-pingTicker.C:
			_ = writeResult(s.ctx, s.writter, methodPing, nil)
		}
	}
}

func (s *serverSession) handlePing(msgID MustString) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer cancel()

	return writeResult(ctx, s.writter, msgID, nil)
}

func (s *serverSession) handleInitialize(
	msgID MustString,
	params initializeParams,
	serverCap ServerCapabilities,
	requiredClientCap ClientCapabilities,
	serverInfo Info,
) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer cancel()

	if params.ProtocolVersion != protocolVersion {
		nErr := fmt.Errorf("protocol version mismatch: %s != %s", params.ProtocolVersion, protocolVersion)
		log.Print(nErr)
		return s.sendError(ctx, jsonRPCInvalidParamsCode, errMsgUnsupportedProtocolVersion, msgID, nErr)
	}

	if requiredClientCap.Roots != nil {
		if params.Capabilities.Roots == nil {
			nErr := fmt.Errorf("insufficient client capabilities: missing required capability 'roots'")
			log.Print(nErr)
			return s.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientClientCapabilities, msgID, nErr)
		}
		if requiredClientCap.Roots.ListChanged {
			if !params.Capabilities.Roots.ListChanged {
				nErr := fmt.Errorf("insufficient client capabilities: missing required capability 'roots.listChanged'")
				log.Print(nErr)
				return s.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientClientCapabilities, msgID, nErr)
			}
		}
	}

	if requiredClientCap.Sampling != nil {
		if params.Capabilities.Sampling == nil {
			nErr := fmt.Errorf("insufficient client capabilities: missing required capability 'sampling'")
			log.Print(nErr)
			return s.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientClientCapabilities, msgID, nErr)
		}
	}

	s.clientCapabilities = params.Capabilities
	s.clientInfo = params.ClientInfo

	return writeResult(ctx, s.writter, msgID, initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    serverCap,
		ServerInfo:      serverInfo,
	})
}

func (s *serverSession) handlePromptsList(
	msgID MustString,
	params promptsListParams,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	ps, err := server.ListPrompts(ctx, params.Cursor, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, ps)
}

func (s *serverSession) handlePromptsGet(
	msgID MustString,
	params promptsGetParams,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	p, err := server.GetPrompt(ctx, params.Name, params.Arguments, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, p)
}

func (s *serverSession) handleCompletePrompt(
	msgID MustString,
	name string,
	argument CompletionArgument,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	result, err := server.CompletesPrompt(ctx, name, argument)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, result)
}

func (s *serverSession) handleResourcesList(
	msgID MustString,
	params resourcesListParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	rs, err := server.ListResources(ctx, params.Cursor, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, rs)
}

func (s *serverSession) handleResourcesRead(
	msgID MustString,
	params resourcesReadParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	r, err := server.ReadResource(ctx, params.URI, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, r)
}

func (s *serverSession) handleResourcesListTemplates(
	msgID MustString,
	params resourcesTemplatesListParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	ts, err := server.ListResourceTemplates(ctx, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, ts)
}

func (s *serverSession) handleResourcesSubscribe(
	msgID MustString,
	params resourcesSubscribeParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	server.SubscribeResource(params.URI)
	s.subscribedResources.Store(params.URI, struct{}{})

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, nil)
}

func (s *serverSession) handleToolsList(
	msgID MustString,
	params toolsListParams,
	server ToolServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	ts, err := server.ListTools(ctx, params.Cursor, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, ts)
}

func (s *serverSession) handleToolsCall(
	msgID MustString,
	params toolsCallParams,
	server ToolServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	result, err := server.CallTool(ctx, params.Name, params.Arguments, params.Meta.ProgressToken)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, result)
}

func (s *serverSession) handleCompleteResource(
	msgID MustString,
	name string,
	argument CompletionArgument,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	result, err := server.CompletesResourceTemplate(ctx, name, argument)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, result)
}

func (s *serverSession) handleNotificationsInitialized() {
	s.initLock.Lock()
	defer s.initLock.Unlock()

	s.initialized = true
}

func (s *serverSession) handleNotificationsCancelled(params notificationsCancelledParams) {
	r, ok := s.clientRequests.Load(params.RequestID)
	if !ok {
		return
	}
	req, _ := r.(clientRequest)

	log.Printf("Cancelled request %s: %s", params.RequestID, params.Reason)
	req.cancel()
}

func (s *serverSession) handleResult(msg jsonRPCMessage) error {
	reqID := string(msg.ID)
	rc, ok := s.serverRequests.Load(reqID)
	if !ok {
		return nil
	}
	resChan, _ := rc.(chan jsonRPCMessage)
	resChan <- msg
	return nil
}

func (s *serverSession) isInitialized() bool {
	s.initLock.RLock()
	defer s.initLock.RUnlock()

	return s.initialized
}

func (s *serverSession) registerRequest() (string, chan jsonRPCMessage) {
	reqID := uuid.New().String()
	resChan := make(chan jsonRPCMessage)
	s.serverRequests.Store(reqID, resChan)
	return reqID, resChan
}

func (s *serverSession) rootsList() (RootList, error) {
	reqID, resChan := s.registerRequest()

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, s.writter, MustString(reqID), methodRootsList, nil); err != nil {
		return RootList{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(s.readTimeout)

	var msg jsonRPCMessage

	select {
	case <-ticker.C:
		return RootList{}, fmt.Errorf("roots list timeout")
	case <-wCtx.Done():
		return RootList{}, wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return RootList{}, msg.Error
	}
	var result RootList
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return RootList{}, err
	}
	return result, nil
}

func (s *serverSession) createSampleMessage(params SamplingParams) (SamplingResult, error) {
	reqID, resChan := s.registerRequest()

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, s.writter, MustString(reqID), methodSamplingCreateMessage, params); err != nil {
		return SamplingResult{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(s.readTimeout)

	var msg jsonRPCMessage

	select {
	case <-ticker.C:
		return SamplingResult{}, fmt.Errorf("sampling message timeout")
	case <-wCtx.Done():
		return SamplingResult{}, wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return SamplingResult{}, msg.Error
	}
	var result SamplingResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return SamplingResult{}, err
	}
	return result, nil
}

func (s *serverSession) sendError(ctx context.Context, code int, message string, msgID MustString, err error) error {
	return writeError(ctx, s.writter, msgID, jsonRPCError{
		Code:    code,
		Message: message,
		Data:    map[string]any{"error": err},
	})
}
