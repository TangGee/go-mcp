package mcp

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

type session struct {
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	writter io.Writer

	clientCapabilities ClientCapabilities
	clientInfo         Info

	writeTimeout time.Duration
	pingInterval time.Duration

	requests            sync.Map // map[requestID]clientRequest
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

func (s *session) listen() {
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

func (s *session) handlePing(msgID MustString) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer cancel()

	return writeResult(ctx, s.writter, msgID, nil)
}

func (s *session) handleInitialize(
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

func (s *session) handlePromptsList(
	msgID MustString,
	params promptsListParams,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
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

func (s *session) handlePromptsGet(
	msgID MustString,
	params promptsGetParams,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
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

func (s *session) handleCompletePrompt(
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

	s.requests.Store(msgID, &clientRequest{
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

func (s *session) handleResourcesList(
	msgID MustString,
	params resourcesListParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
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

func (s *session) handleResourcesRead(
	msgID MustString,
	params resourcesReadParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
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

func (s *session) handleResourcesListTemplates(
	msgID MustString,
	params resourcesTemplatesListParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
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

func (s *session) handleResourcesSubscribe(
	msgID MustString,
	params resourcesSubscribeParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
		ctx:    ctx,
		cancel: cancel,
	})

	server.SubscribeResource(params.URI)
	s.subscribedResources.Store(params.URI, struct{}{})

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, nil)
}

func (s *session) handleToolsList(
	msgID MustString,
	params toolsListParams,
	server ToolServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
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

func (s *session) handleToolsCall(
	msgID MustString,
	params toolsCallParams,
	server ToolServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.requests.Store(msgID, &clientRequest{
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

func (s *session) handleCompleteResource(
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

	s.requests.Store(msgID, &clientRequest{
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

func (s *session) handleNotificationsInitialized() {
	s.initLock.Lock()
	defer s.initLock.Unlock()

	s.initialized = true
}

func (s *session) handleNotificationsCancelled(params notificationsCancelledParams) {
	s.initLock.Lock()
	defer s.initLock.Unlock()

	r, ok := s.requests.Load(params.RequestID)
	if !ok {
		return
	}
	req, _ := r.(clientRequest)

	log.Printf("Cancelled request %s: %s", params.RequestID, params.Reason)
	req.cancel()
}

func (s *session) isInitialized() bool {
	s.initLock.RLock()
	defer s.initLock.RUnlock()

	return s.initialized
}

func (s *session) sendError(ctx context.Context, code int, message string, msgID MustString, err error) error {
	return writeError(ctx, s.writter, msgID, jsonRPCError{
		Code:    code,
		Message: message,
		Data:    map[string]any{"error": err},
	})
}
