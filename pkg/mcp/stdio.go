package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// StdIO implements a standard input/output transport layer for MCP communication.
// It provides bidirectional message passing using stdin/stdout or similar io.Reader/io.Writer
// pairs, with JSON-RPC message encoding.
//
// StdIO manages a single persistent session and handles message routing through
// internal channels. It provides non-blocking error reporting and graceful shutdown
// capabilities through its channel-based architecture.
//
// The transport layer maintains a single persistent session identified by "1" and
// processes messages sequentially through its internal channels. Error handling is
// managed through a dedicated error channel that provides non-blocking error reporting.
type StdIO struct {
	reader io.Reader
	writer io.Writer

	messagesChan chan SessionMsgWithErrs
	errsChan     chan error
	closeChan    chan struct{}
}

// NewStdIO creates a new standard IO transport instance using the provided reader and writer.
// The reader is typically os.Stdin and writer is typically os.Stdout, though any io.Reader
// and io.Writer implementations can be used for testing or custom IO scenarios.
//
// It initializes internal channels for message passing, error handling, and shutdown
// coordination. The transport is ready for use immediately after creation but requires
// Start() to be called to begin processing messages.
// and io.Writer implementations can be used for testing or custom IO scenarios.
func NewStdIO(reader io.Reader, writer io.Writer) StdIO {
	return StdIO{
		reader:       reader,
		writer:       writer,
		messagesChan: make(chan SessionMsgWithErrs),
		errsChan:     make(chan error),
		closeChan:    make(chan struct{}),
	}
}

// Start begins processing input messages from the reader in a blocking manner.
// It continuously reads JSON-RPC messages line by line, unmarshals them, and
// forwards them to the message channel for processing.
//
// The processing loop continues until either the reader is exhausted or Close()
// is called. Any unmarshaling or processing errors are sent to the error channel.
//
// This method should typically be called in a separate goroutine as it blocks
// until completion or shutdown.
func (s StdIO) Start() {
	scanner := bufio.NewScanner(s.reader)
	for scanner.Scan() {
		select {
		case <-s.closeChan:
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg JSONRPCMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			s.logError(fmt.Errorf("failed to unmarshal message: %w", err))
			continue
		}

		errs := make(chan error)
		s.messagesChan <- SessionMsgWithErrs{
			SessionID: "1",
			Msg:       msg,
			Errs:      errs,
		}

		if err := <-errs; err != nil {
			s.logError(fmt.Errorf("failed to handle message: %w", err))
		}
	}

	if err := scanner.Err(); err != nil {
		s.logError(fmt.Errorf("failed to read messages: %w", err))
	}
}

// Send writes a JSON-RPC message to the writer with context cancellation support.
// It marshals the message to JSON, appends a newline, and writes it to the underlying writer.
//
// The context allows for cancellation of long-running write operations. If the context
// is cancelled before the write completes, the operation is abandoned and ctx.Err() is returned.
//
// Returns an error if marshaling fails, the write operation fails, or the context is cancelled.
func (s StdIO) Send(ctx context.Context, msg SessionMsg) error {
	msgBs, err := json.Marshal(msg.Msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	msgBs = append(msgBs, '\n')

	errs := make(chan error)

	go func() {
		_, err = s.writer.Write(msgBs)
		if err != nil {
			errs <- fmt.Errorf("failed to write message: %w", err)
			return
		}
		errs <- nil
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-errs:
	}

	return err
}

// SessionMessages returns a receive-only channel that provides access to incoming
// messages with their associated error channels. Each message is wrapped in a
// SessionMsgWithErrs struct that includes the session ID and an error channel
// for reporting processing results.
//
// The channel is closed when the StdIO instance is closed via Close().
func (s StdIO) SessionMessages() <-chan SessionMsgWithErrs {
	return s.messagesChan
}

// Close terminates the StdIO transport by closing the message and control channels.
// This will cause the Start() method to exit if it is running and prevent any
// new messages from being processed.
//
// After Close() is called, the transport cannot be reused and a new instance
// should be created if needed.
func (s StdIO) Close() {
	close(s.closeChan)
}

// Sessions returns a receive-only channel that provides the single session context
// used by this transport. Since StdIO only supports a single session, this method
// sends one SessionCtx with ID "1" and a background context.
//
// The returned channel should be consumed to prevent goroutine leaks.
func (s StdIO) Sessions() <-chan SessionCtx {
	sessions := make(chan SessionCtx)
	go func() {
		sessions <- SessionCtx{
			Ctx: context.Background(),
			ID:  "1",
		}
	}()

	return sessions
}

// StartSession initializes a new session for the StdIO transport. Since this
// implementation only supports a single session, it always returns the session
// ID "1" with no error.
//
// This method is part of the Transport interface but has limited utility in
// the StdIO implementation due to its single-session nature.
func (s StdIO) StartSession() (string, error) {
	return "1", nil
}

// Errors returns a receive-only channel that provides access to transport-level
// errors. These may include message parsing errors, write failures, or other
// operational issues encountered during transport operation.
//
// The channel is non-blocking and may drop errors if not consumed quickly enough.
func (s StdIO) Errors() <-chan error {
	return s.errsChan
}

func (s StdIO) logError(err error) {
	select {
	case s.errsChan <- err:
	default:
	}
}
