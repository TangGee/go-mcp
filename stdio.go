package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
)

// StdIO implements a standard input/output transport layer for MCP communication using
// JSON-RPC message encoding over stdin/stdout or similar io.Reader/io.Writer pairs. It
// provides a single persistent session identified as "1" and handles bidirectional message
// passing through internal channels, processing messages sequentially.
//
// The transport layer maintains internal state through its embedded stdIOSession and can
// be used as either ServerTransport or ClientTransport. Proper initialization requires
// using the NewStdIO constructor function to create new instances.
//
// Resources must be properly released by calling Close when the StdIO instance is no
// longer needed.
type StdIO struct {
	sess stdIOSession
}

type stdIOSession struct {
	reader io.ReadCloser
	writer io.WriteCloser
	logger *slog.Logger

	// done signals session termination to all goroutines
	done chan struct{}
	// closed is used to ensure proper cleanup sequencing
	closed chan struct{}
}

// NewStdIO creates a new StdIO instance configured with the provided reader and writer.
// The instance is initialized with default logging and required internal communication
// channels.
func NewStdIO(reader io.ReadCloser, writer io.WriteCloser) StdIO {
	return StdIO{
		sess: stdIOSession{
			reader: reader,
			writer: writer,
			logger: slog.Default(),
			done:   make(chan struct{}),
			closed: make(chan struct{}),
		},
	}
}

// Sessions implements the ServerTransport interface by providing an iterator that yields
// a single persistent session. This session remains active throughout the lifetime of
// the StdIO instance.
func (s StdIO) Sessions() iter.Seq[Session] {
	return func(yield func(Session) bool) {
		yield(s.sess)
	}
}

// Send implements the ClientTransport interface by transmitting a JSON-RPC message to
// the server through the established session. The context can be used to control the
// transmission timeout.
func (s StdIO) Send(ctx context.Context, msg JSONRPCMessage) error {
	return s.sess.Send(ctx, msg)
}

// StartSession implements the ClientTransport interface by initializing a new session
// and returning an iterator for receiving server messages. The ready channel is closed
// immediately to indicate session establishment.
func (s StdIO) StartSession(_ context.Context, ready chan<- error) (iter.Seq[JSONRPCMessage], error) {
	close(ready)
	return s.sess.Messages(), nil
}

// Close releases all resources associated with the StdIO instance, including its
// underlying reader and writer connections.
func (s StdIO) Close() {
	s.sess.close()
}

func (s stdIOSession) ID() string {
	return "1"
}

func (s stdIOSession) Send(ctx context.Context, msg JSONRPCMessage) error {
	msgBs, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	// We append newline to maintain message framing protocol
	msgBs = append(msgBs, '\n')

	errs := make(chan error, 1)

	// We use a goroutine for writing to prevent blocking on slow writers
	// while still respecting context cancellation
	go func() {
		_, err = s.writer.Write(msgBs)
		if err != nil {
			errs <- fmt.Errorf("failed to write message: %w", err)
			return
		}
		errs <- nil
	}()

	// We prioritize context cancellation and session termination over write completion
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return nil
	case err := <-errs:
		return err
	}
}

func (s stdIOSession) Messages() iter.Seq[JSONRPCMessage] {
	return func(yield func(JSONRPCMessage) bool) {
		defer close(s.closed)

		scanner := bufio.NewScanner(s.reader)
		for scanner.Scan() {
			// We check for session termination between each message
			select {
			case <-s.done:
				return
			default:
			}

			line := scanner.Text()
			if line == "" {
				continue
			}

			var msg JSONRPCMessage
			err := json.Unmarshal([]byte(line), &msg)
			if err != nil {
				s.logger.Error("failed to unmarshal message", "err", err)
				continue
			}

			// We stop iteration if yield returns false
			if !yield(msg) {
				return
			}
		}
		// We ignore ErrClosedPipe as it's an expected error during shutdown
		if scanner.Err() != nil && !errors.Is(scanner.Err(), io.ErrClosedPipe) {
			s.logger.Error("scan error", "err", scanner.Err())
		}
	}
}

func (s stdIOSession) close() {
	if err := s.reader.Close(); err != nil {
		s.logger.Error("failed to close reader", "err", err)
	}
	if err := s.writer.Close(); err != nil {
		s.logger.Error("failed to close writer", "err", err)
	}

	// We signal termination and wait for Messages goroutine to complete
	close(s.done)
	<-s.closed
}
