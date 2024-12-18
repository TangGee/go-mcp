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
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	writter io.Writer

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
			if err := s.ping(); err != nil {
				log.Print(err)
			}
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

	s.clientRequests.Store(msgID, &request{
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

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
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

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
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

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
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

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
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

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
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

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	server.SubscribeResource(params.URI)
	s.subscribedResources.Store(params.URI, struct{}{})

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, nil)
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

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
	result, err := server.CompletesResourceTemplate(ctx, name, argument)
	if err != nil {
		return s.sendError(ctx, jsonRPCInternalErrorCode, errMsgInternalError, msgID, err)
	}

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	return writeResult(wCtx, s.writter, msgID, result)
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

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
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

	s.clientRequests.Store(msgID, &request{
		ctx:    ctx,
		cancel: cancel,
	})

	ctx = ctxWithSessionID(ctx, s.id)
	result, err := server.CallTool(ctx, params.Name, params.Arguments, params.Meta.ProgressToken)
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
	req, _ := r.(request)

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

func (s *serverSession) listRoots() (RootList, error) {
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

func (s *serverSession) ping() error {
	reqID, resChan := s.registerRequest()

	wCtx, wCancel := context.WithTimeout(s.ctx, s.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, s.writter, MustString(reqID), methodPing, nil); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(s.readTimeout)

	var msg jsonRPCMessage

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
	return writeError(ctx, s.writter, msgID, jsonRPCError{
		Code:    code,
		Message: message,
		Data:    map[string]any{"error": err},
	})
}

func (c *clientSession) listen() {
	pingTicker := time.NewTicker(c.pingInterval)

	for {
		select {
		case <-c.ctx.Done():
			c.stopChan <- c.id
			return
		case <-c.rootsListChan:
			_ = writeResult(c.ctx, c.writter, methodNotificationsRootsListChanged, nil)
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

func (c *clientSession) handleResult(msg jsonRPCMessage) error {
	reqID := string(msg.ID)
	rc, ok := c.clientRequests.Load(reqID)
	if !ok {
		return nil
	}
	resChan, _ := rc.(chan jsonRPCMessage)
	resChan <- msg
	return nil
}

func (c *clientSession) registerRequest() (string, chan jsonRPCMessage) {
	reqID := uuid.New().String()
	resChan := make(chan jsonRPCMessage)
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

	var msg jsonRPCMessage

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

	if err := c.checkCapabilities(ctx, msg.ID, result, requiredServerCap); err != nil {
		return err
	}

	c.initLock.Lock()
	defer c.initLock.Unlock()
	c.initialized = true

	return nil
}

func (c *clientSession) listPrompts(cursor string, progressToken MustString) (PromptList, error) {
	reqID, resChan := c.registerRequest()

	wCtx, wCancel := context.WithTimeout(c.ctx, c.writeTimeout)
	defer wCancel()

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodPromptsList, promptsListParams{
		Cursor: cursor,
		Meta:   paramsMeta{ProgressToken: progressToken},
	}); err != nil {
		return PromptList{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg jsonRPCMessage

	select {
	case <-ticker.C:
		return PromptList{}, fmt.Errorf("list prompts timeout")
	case <-wCtx.Done():
		return PromptList{}, wCtx.Err()
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

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodPromptsGet, promptsGetParams{
		Name:      name,
		Arguments: arguments,
		Meta:      paramsMeta{ProgressToken: progressToken},
	}); err != nil {
		return Prompt{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg jsonRPCMessage

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

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodCompletionComplete, completionCompleteParams{
		Ref: completionCompleteRef{
			Type: "ref/prompt",
			Name: name,
		},
		Argument: arg,
	}); err != nil {
		return CompletionResult{}, err
	}

	ticker := time.NewTicker(c.readTimeout)
	defer ticker.Stop()

	var msg jsonRPCMessage

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

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodResourcesList, resourcesListParams{
		Cursor: cursor,
		Meta:   paramsMeta{ProgressToken: progressToken},
	}); err != nil {
		return ResourceList{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg jsonRPCMessage

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

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodResourcesRead, resourcesReadParams{
		URI:  uri,
		Meta: paramsMeta{ProgressToken: progressToken},
	}); err != nil {
		return Resource{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg jsonRPCMessage

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

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodResourcesTemplatesList, resourcesTemplatesListParams{
		Meta: paramsMeta{ProgressToken: progressToken},
	}); err != nil {
		return nil, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg jsonRPCMessage

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

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodCompletionComplete, completionCompleteParams{
		Ref: completionCompleteRef{
			Type: "ref/resource",
			URI:  uri,
		},
		Argument: arg,
	}); err != nil {
		return CompletionResult{}, err
	}

	ticker := time.NewTicker(c.readTimeout)
	defer ticker.Stop()

	var msg jsonRPCMessage

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

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodResourcesSubscribe, resourcesSubscribeParams{
		URI: uri,
	}); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg jsonRPCMessage

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

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodToolsList, toolsListParams{
		Cursor: cursor,
		Meta:   paramsMeta{ProgressToken: progressToken},
	}); err != nil {
		return ToolList{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg jsonRPCMessage

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

	if err := writeParams(wCtx, c.writter, MustString(reqID), methodToolsCall, toolsCallParams{
		Name:      name,
		Arguments: arguments,
		Meta:      paramsMeta{ProgressToken: progressToken},
	}); err != nil {
		return ToolResult{}, fmt.Errorf("failed to write message: %w", err)
	}

	ticker := time.NewTicker(c.readTimeout)

	var msg jsonRPCMessage

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

	var msg jsonRPCMessage

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

func (c *clientSession) checkCapabilities(
	ctx context.Context,
	msgID MustString,
	result initializeResult,
	requiredServerCap ServerCapabilities,
) error {
	if requiredServerCap.Prompts != nil {
		if result.Capabilities.Prompts == nil {
			nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'prompts'")
			log.Print(nErr)
			return c.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientServerCapabilities, msgID, nErr)
		}
		if requiredServerCap.Prompts.ListChanged {
			if !result.Capabilities.Prompts.ListChanged {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'prompts.listChanged'")
				log.Print(nErr)
				return c.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientServerCapabilities, msgID, nErr)
			}
		}
	}

	if requiredServerCap.Resources != nil {
		if result.Capabilities.Resources == nil {
			nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'resources'")
			log.Print(nErr)
			return c.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientServerCapabilities, msgID, nErr)
		}
		if requiredServerCap.Resources.ListChanged {
			if !result.Capabilities.Resources.ListChanged {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'resources.listChanged'")
				log.Print(nErr)
				return c.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientServerCapabilities, msgID, nErr)
			}
		}
		if requiredServerCap.Resources.Subscribe {
			if !result.Capabilities.Resources.Subscribe {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'resources.subscribe'")
				log.Print(nErr)
				return c.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientServerCapabilities, msgID, nErr)
			}
		}
	}

	if requiredServerCap.Tools != nil {
		if result.Capabilities.Tools == nil {
			nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'tools'")
			log.Print(nErr)
			return c.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientServerCapabilities, msgID, nErr)
		}
		if requiredServerCap.Tools.ListChanged {
			if !result.Capabilities.Tools.ListChanged {
				nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'tools.listChanged'")
				log.Print(nErr)
				return c.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientServerCapabilities, msgID, nErr)
			}
		}
	}

	if requiredServerCap.Logging != nil {
		nErr := fmt.Errorf("insufficient server capabilities: missing required capability 'logging'")
		log.Print(nErr)
		return c.sendError(ctx, jsonRPCInvalidParamsCode, errMsgInsufficientServerCapabilities, msgID, nErr)
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
