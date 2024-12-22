package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

type serverSession struct {
	id     string
	ctx    context.Context
	cancel context.CancelFunc
	writer io.Writer

	writeTimeout time.Duration
	readTimeout  time.Duration
	pingInterval time.Duration

	clientRequests      sync.Map // map[requestID]request
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

type clientSession struct {
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	writter io.Writer

	writeTimeout time.Duration
	readTimeout  time.Duration
	pingInterval time.Duration

	serverRequests sync.Map // map[requestID]request
	clientRequests sync.Map // map[requestID]chan jsonRPCMessage

	rootsListChan chan struct{}
	stopChan      chan<- string

	initLock    sync.RWMutex
	initialized bool
}

type request struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type sessionIDKeyType string

const sessionIDKey sessionIDKeyType = "sessionID"

var (
	errInvalidJSON     = errors.New("invalid json")
	errSessionNotFound = errors.New("session not found")
)

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
	for {
		select {
		case <-s.ctx.Done():
			s.stopChan <- s.id
			return
		case <-s.promptsListChan:
			_ = writeNotifications(s.ctx, s.writer, methodNotificationsPromptsListChanged, nil)
		case <-s.resourcesListChan:
			_ = writeNotifications(s.ctx, s.writer, methodNotificationsResourcesListChanged, nil)
		case uri := <-s.resourcesSubscribeChan:
			_, ok := s.subscribedResources.Load(uri)
			if !ok {
				continue
			}
			_ = writeNotifications(s.ctx, s.writer, methodNotificationsResourcesUpdated, notificationsResourcesUpdatedParams{
				URI: uri,
			})
		case <-s.toolsListChan:
			_ = writeNotifications(s.ctx, s.writer, methodNotificationsToolsListChanged, nil)
		case params := <-s.logChan:
			_ = writeNotifications(s.ctx, s.writer, methodNotificationsMessage, params)
		case params := <-s.progressChan:
			_ = writeNotifications(s.ctx, s.writer, methodNotificationsProgress, params)
		}
	}
}

func (s *serverSession) pings() {
	pingTicker := time.NewTicker(s.pingInterval)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-pingTicker.C:
			if err := s.ping(); err != nil {
				log.Print(err)
			}
		}
	}
}

func (s *serverSession) handlePing(msgID MustString) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer cancel()

	return writeResult(ctx, s.writer, msgID, nil)
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

	return writeResult(ctx, s.writer, msgID, initializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    serverCap,
		ServerInfo:      serverInfo,
	})
}

func (s *serverSession) handlePromptsList(
	msgID MustString,
	params PromptsListParams,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ps, err := server.ListPrompts(ctx, params)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writer, msgID, ps)
}

func (s *serverSession) handlePromptsGet(
	msgID MustString,
	params PromptsGetParams,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
	p, err := server.GetPrompt(ctx, params)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writer, msgID, p)
}

func (s *serverSession) handleCompletePrompt(
	msgID MustString,
	params CompletionCompleteParams,
	server PromptServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
	result, err := server.CompletesPrompt(ctx, params)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writer, msgID, result)
}

func (s *serverSession) handleResourcesList(
	msgID MustString,
	params ResourcesListParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
	rs, err := server.ListResources(ctx, params)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writer, msgID, rs)
}

func (s *serverSession) handleResourcesRead(
	msgID MustString,
	params ResourcesReadParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
	r, err := server.ReadResource(ctx, params)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writer, msgID, r)
}

func (s *serverSession) handleResourcesListTemplates(
	msgID MustString,
	params ResourcesTemplatesListParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
	ts, err := server.ListResourceTemplates(ctx, params)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writer, msgID, ts)
}

func (s *serverSession) handleResourcesSubscribe(
	msgID MustString,
	params ResourcesSubscribeParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	server.SubscribeResource(params)
	s.subscribedResources.Store(params.URI, struct{}{})

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writer, msgID, nil)
}

func (s *serverSession) handleCompleteResource(
	msgID MustString,
	params CompletionCompleteParams,
	server ResourceServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
	result, err := server.CompletesResourceTemplate(ctx, params)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writer, msgID, result)
}

func (s *serverSession) handleToolsList(
	msgID MustString,
	params ToolsListParams,
	server ToolServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
	ts, err := server.ListTools(ctx, params)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writer, msgID, ts)
}

func (s *serverSession) handleToolsCall(
	msgID MustString,
	params ToolsCallParams,
	server ToolServer,
) error {
	if !s.isInitialized() {
		return nil
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
	result, err := server.CallTool(ctx, params)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writer, msgID, result)
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
	req, _ := r.(request)

	log.Printf("Cancelled request %s: %s", params.RequestID, params.Reason)
	req.cancel()
}

func (s *serverSession) handleResult(msg JSONRPCMessage) error {
	reqID := string(msg.ID)
	rc, ok := s.serverRequests.Load(reqID)
	if !ok {
		return nil
	}
	resChan, _ := rc.(chan JSONRPCMessage)
	resChan <- msg
	return nil
}

func (s *serverSession) isInitialized() bool {
	s.initLock.RLock()
	defer s.initLock.RUnlock()

	return s.initialized
}

func (s *serverSession) registerRequest() (string, chan JSONRPCMessage) {
	reqID := uuid.New().String()
	resChan := make(chan JSONRPCMessage)
	s.serverRequests.Store(reqID, resChan)
	return reqID, resChan
}

func (s *serverSession) listRoots() (RootList, error) {
	reqID, resChan := s.registerRequest()

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, s.writer, MustString(reqID), MethodRootsList, nil); err != nil {
		return RootList{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(s.readTimeout)

	var msg JSONRPCMessage

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

	if err := writeParams(wCtx, s.writer, MustString(reqID), MethodSamplingCreateMessage, params); err != nil {
		return SamplingResult{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(s.readTimeout)

	var msg JSONRPCMessage

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

func (s *serverSession) ping() error {
	reqID, resChan := s.registerRequest()

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, s.writer, MustString(reqID), methodPing, nil); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(s.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return fmt.Errorf("ping timeout")
	case <-wCtx.Done():
		return wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return msg.Error
	}

	return nil
}

func (s *serverSession) sendError(ctx context.Context, code int, message string, msgID MustString, err error) error {
	return writeError(ctx, s.writer, msgID, jsonRPCError{
		Code:    code,
		Message: message,
		Data:    map[string]any{"error": err.Error()},
	})
}

func (c *clientSession) listen() {
	for {
		select {
		case <-c.ctx.Done():
			c.stopChan <- c.id
			return
		case <-c.rootsListChan:
			_ = writeNotifications(c.ctx, c.writter, methodNotificationsRootsListChanged, nil)
		}
	}
}

func (c *clientSession) pings() {
	pingTicker := time.NewTicker(c.pingInterval)

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-pingTicker.C:
			if err := c.ping(); err != nil {
				log.Print(err)
			}
		}
	}
}

func (c *clientSession) handlePing(msgID MustString) error {
	ctx, cancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer cancel()

	return writeResult(ctx, c.writter, msgID, nil)
}

func (c *clientSession) handleRootsList(msgID MustString, handler RootsListHandler) error {
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	c.serverRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, c.id)
	roots, err := handler.RootsList(ctx)
	if err != nil {
		return c.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, c.writter, msgID, roots)
}

func (c *clientSession) handleSamplingCreateMessage(
	msgID MustString,
	params SamplingParams,
	handler SamplingHandler,
) error {
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	c.serverRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, c.id)
	result, err := handler.CreateSampleMessage(ctx, params)
	if err != nil {
		return c.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, c.writter, msgID, result)
}

func (c *clientSession) handleNotificationsCancelled(params notificationsCancelledParams) {
	r, ok := c.serverRequests.Load(params.RequestID)
	if !ok {
		return
	}
	req, _ := r.(request)

	log.Printf("Cancelled request %s: %s", params.RequestID, params.Reason)
	req.cancel()
}

func (c *clientSession) handleResult(msg JSONRPCMessage) error {
	reqID := string(msg.ID)
	rc, ok := c.clientRequests.Load(reqID)
	if !ok {
		return nil
	}
	resChan, _ := rc.(chan JSONRPCMessage)
	resChan <- msg
	return nil
}

func (c *clientSession) registerRequest() (string, chan JSONRPCMessage) {
	reqID := uuid.New().String()
	resChan := make(chan JSONRPCMessage)
	c.clientRequests.Store(reqID, resChan)
	return reqID, resChan
}

func (c *clientSession) initialize(
	capabilities ClientCapabilities,
	info Info,
	requiredServerCap ServerCapabilities,
) error {
	reqID, reqChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodInitialize, initializeParams{
		ProtocolVersion: protocolVersion,
		Capabilities:    capabilities,
		ClientInfo:      info,
	}); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return fmt.Errorf("initialize timeout")
	case <-wCtx.Done():
		return wCtx.Err()
	case msg = <-reqChan:
	}

	if msg.Error != nil {
		return msg.Error
	}
	var result initializeResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer cancel()

	if result.ProtocolVersion != protocolVersion {
		nErr := fmt.Errorf("protocol version mismatch: %s != %s", result.ProtocolVersion, protocolVersion)
		log.Print(nErr)
		return c.sendError(ctx, jsonRPCInvalidParamsCode, errMsgUnsupportedProtocolVersion, msg.ID, nErr)
	}

	if err := c.checkCapabilities(result, requiredServerCap); err != nil {
		return err
	}

	c.initLock.Lock()
	defer c.initLock.Unlock()
	c.initialized = true

	return writeNotifications(ctx, c.writter, methodNotificationsInitialized, nil)
}

func (c *clientSession) listPrompts(cursor string, progressToken MustString) (PromptList, error) {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), MethodPromptsList, PromptsListParams{
		Cursor: cursor,
		Meta:   ParamsMeta{ProgressToken: progressToken},
	}); err != nil {
		return PromptList{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return PromptList{}, fmt.Errorf("list prompts timeout")
	case <-wCtx.Done():
		err := wCtx.Err()
		if !errors.Is(err, context.Canceled) {
			return PromptList{}, err
		}
		cCtx, cCancel := context.WithTimeout(c.ctx, c.writeTimeout)
		defer cCancel()
		return PromptList{}, writeNotifications(cCtx, c.writter, methodNotificationsCancelled, notificationsCancelledParams{
			RequestID: reqID,
			Reason:    userCancelledReason,
		})
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return PromptList{}, msg.Error
	}
	var result PromptList
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return PromptList{}, err
	}
	return result, nil
}

func (c *clientSession) getPrompt(name string, arguments map[string]any, progressToken MustString) (Prompt, error) {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), MethodPromptsGet, PromptsGetParams{
		Name:      name,
		Arguments: arguments,
		Meta:      ParamsMeta{ProgressToken: progressToken},
	}); err != nil {
		return Prompt{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return Prompt{}, fmt.Errorf("get prompt timeout")
	case <-wCtx.Done():
		return Prompt{}, wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return Prompt{}, msg.Error
	}
	var result Prompt
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return Prompt{}, err
	}
	return result, nil
}

func (c *clientSession) completesPrompt(name string, arg CompletionArgument) (CompletionResult, error) {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithCancel(c.ctx)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), MethodCompletionComplete, CompletionCompleteParams{
		Ref: CompletionCompleteRef{
			Type: CompletionRefPrompt,
			Name: name,
		},
		Argument: arg,
	}); err != nil {
		return CompletionResult{}, err
	}

	ticker := time.NewTicker(c.readTimeout)
	defer ticker.Stop()

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return CompletionResult{}, fmt.Errorf("complete prompt timeout")
	case <-wCtx.Done():
		return CompletionResult{}, wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return CompletionResult{}, msg.Error
	}
	var result CompletionResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return CompletionResult{}, err
	}
	return result, nil
}

func (c *clientSession) listResources(cursor string, progressToken MustString) (ResourceList, error) {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), MethodResourcesList, ResourcesListParams{
		Cursor: cursor,
		Meta:   ParamsMeta{ProgressToken: progressToken},
	}); err != nil {
		return ResourceList{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return ResourceList{}, fmt.Errorf("list resources timeout")
	case <-wCtx.Done():
		return ResourceList{}, wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return ResourceList{}, msg.Error
	}
	var result ResourceList
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return ResourceList{}, err
	}
	return result, nil
}

func (c *clientSession) readResource(uri string, progressToken MustString) (Resource, error) {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), MethodResourcesRead, ResourcesReadParams{
		URI:  uri,
		Meta: ParamsMeta{ProgressToken: progressToken},
	}); err != nil {
		return Resource{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return Resource{}, fmt.Errorf("read resource timeout")
	case <-wCtx.Done():
		return Resource{}, wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return Resource{}, msg.Error
	}
	var result Resource
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return Resource{}, err
	}
	return result, nil
}

func (c *clientSession) listResourceTemplates(progressToken MustString) ([]ResourceTemplate, error) {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), MethodResourcesTemplatesList, ResourcesTemplatesListParams{
		Meta: ParamsMeta{ProgressToken: progressToken},
	}); err != nil {
		return nil, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return nil, fmt.Errorf("list resource templates timeout")
	case <-wCtx.Done():
		return nil, wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return nil, msg.Error
	}
	var result []ResourceTemplate
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *clientSession) completesResourceTemplate(uri string, arg CompletionArgument) (CompletionResult, error) {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithCancel(c.ctx)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), MethodCompletionComplete, CompletionCompleteParams{
		Ref: CompletionCompleteRef{
			Type: CompletionRefResource,
			URI:  uri,
		},
		Argument: arg,
	}); err != nil {
		return CompletionResult{}, err
	}

	ticker := time.NewTicker(c.readTimeout)
	defer ticker.Stop()

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return CompletionResult{}, fmt.Errorf("complete prompt timeout")
	case <-wCtx.Done():
		return CompletionResult{}, wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return CompletionResult{}, msg.Error
	}
	var result CompletionResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return CompletionResult{}, err
	}
	return result, nil
}

func (c *clientSession) subscribeResource(uri string) error {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), MethodResourcesSubscribe, ResourcesSubscribeParams{
		URI: uri,
	}); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return fmt.Errorf("subscribe resource timeout")
	case <-wCtx.Done():
		return fmt.Errorf("subscribe resource canceled")
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return fmt.Errorf("subscribe resource error: %w", msg.Error)
	}

	return nil
}

func (c *clientSession) listTools(cursor string, progressToken MustString) (ToolList, error) {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), MethodToolsList, ToolsListParams{
		Cursor: cursor,
		Meta:   ParamsMeta{ProgressToken: progressToken},
	}); err != nil {
		return ToolList{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return ToolList{}, fmt.Errorf("list tools timeout")
	case <-wCtx.Done():
		return ToolList{}, wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return ToolList{}, msg.Error
	}
	var result ToolList
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return ToolList{}, err
	}
	return result, nil
}

func (c *clientSession) callTool(name string, arguments map[string]any, progressToken MustString) (ToolResult, error) {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), MethodToolsCall, ToolsCallParams{
		Name:      name,
		Arguments: arguments,
		Meta:      ParamsMeta{ProgressToken: progressToken},
	}); err != nil {
		return ToolResult{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return ToolResult{}, fmt.Errorf("call tool timeout")
	case <-wCtx.Done():
		return ToolResult{}, wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return ToolResult{}, msg.Error
	}
	var result ToolResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return ToolResult{}, err
	}
	return result, nil
}

func (c *clientSession) ping() error {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodPing, nil); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg JSONRPCMessage

	select {
	case <-ticker.C:
		return fmt.Errorf("ping timeout")
	case <-wCtx.Done():
		return wCtx.Err()
	case msg = <-resChan:
	}

	if msg.Error != nil {
		return msg.Error
	}

	return nil
}

func (c *clientSession) checkCapabilities(result initializeResult, requiredServerCap ServerCapabilities) error {
	if requiredServerCap.Prompts != nil {
		if result.Capabilities.Prompts == nil {
			nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'prompts'")
			log.Print(nErr)
			return nErr
		}
		if requiredServerCap.Prompts.ListChanged {
			if !result.Capabilities.Prompts.ListChanged {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'prompts.listChanged'")
				log.Print(nErr)
				return nErr
			}
		}
	}

	if requiredServerCap.Resources != nil {
		if result.Capabilities.Resources == nil {
			nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'resources'")
			log.Print(nErr)
			return nErr
		}
		if requiredServerCap.Resources.ListChanged {
			if !result.Capabilities.Resources.ListChanged {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'resources.listChanged'")
				log.Print(nErr)
				return nErr
			}
		}
		if requiredServerCap.Resources.Subscribe {
			if !result.Capabilities.Resources.Subscribe {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'resources.subscribe'")
				log.Print(nErr)
				return nErr
			}
		}
	}

	if requiredServerCap.Tools != nil {
		if result.Capabilities.Tools == nil {
			nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'tools'")
			log.Print(nErr)
			return nErr
		}
		if requiredServerCap.Tools.ListChanged {
			if !result.Capabilities.Tools.ListChanged {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'tools.listChanged'")
				log.Print(nErr)
				return nErr
			}
		}
	}

	if requiredServerCap.Logging != nil {
		nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'logging'")
		log.Print(nErr)
		return nErr
	}

	return nil
}

func (c *clientSession) sendError(ctx context.Context, code int, message string, msgID MustString, err error) error {
	return writeError(ctx, c.writter, msgID, jsonRPCError{
		Code:    code,
		Message: message,
		Data:    map[string]any{"error": err},
	})
}
