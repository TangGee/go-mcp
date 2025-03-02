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
	"strings"

	"github.com/google/uuid"
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
	sess   stdIOSession
	closed chan struct{}
}

type stdIOSession struct {
	reader io.Reader
	writer io.Writer
	logger *slog.Logger

	writeMessages chan stdIOMessage
	done          chan struct{}
	readClosed    chan struct{}
	writeClosed   chan struct{}
}

type stdIOMessage struct {
	msg  []byte
	errs chan error
}

// NewStdIO creates a new StdIO instance configured with the provided reader and writer.
// The instance is initialized with default logging and required internal communication
// channels.
func NewStdIO(reader io.Reader, writer io.Writer) StdIO {
	return StdIO{
		sess: stdIOSession{
			reader:        reader,
			writer:        writer,
			logger:        slog.Default(),
			writeMessages: make(chan stdIOMessage),
			done:          make(chan struct{}),
			readClosed:    make(chan struct{}),
			writeClosed:   make(chan struct{}),
		},
		closed: make(chan struct{}),
	}
}

// Sessions implements the ServerTransport interface by providing an iterator that yields
// a single persistent session. This session remains active throughout the lifetime of
// the StdIO instance.
func (s StdIO) Sessions() iter.Seq[Session] {
	return func(yield func(Session) bool) {
		defer close(s.closed)

		go s.sess.processWriteMessages()

		// StdIO only supports a single session, so we yield it and wait until it's done.
		yield(s.sess)
		<-s.sess.done
	}
}

// Shutdown implements the ServerTransport interface by closing the session.
func (s StdIO) Shutdown(ctx context.Context) error {
	// Wait for Sessions loop to breaks.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closed:
	}
	return nil
}

// StartSession implements the ClientTransport interface by initializing a new session
// and returning an iterator for receiving server messages. The ready channel is closed
// immediately to indicate session establishment.
func (s StdIO) StartSession(_ context.Context) (Session, error) {
	go s.sess.processWriteMessages()
	return s.sess, nil
}

func (s stdIOSession) ID() string {
	return uuid.New().String()
}

func (s stdIOSession) Send(ctx context.Context, msg JSONRPCMessage) error {
	msgBs, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	// Append newline to maintain message framing protocol
	msgBs = append(msgBs, '\n')

	ioMsg := stdIOMessage{
		msg:  msgBs,
		errs: make(chan error, 1),
	}

	// Queue the message for sending to avoid race in the StdIO library.
	select {
	case <-ctx.Done():
		s.logger.Error("failed to feed writeMessages channel", slog.String("err", ctx.Err().Error()))
		return ctx.Err()
	case <-s.done:
		s.logger.Warn("session is closed while feeding writeMessages channel", slog.String("message", string(msgBs)))
		return nil
	case s.writeMessages <- ioMsg:
	}

	// Wait for the resulting error channel to receive the error.
	select {
	case err := <-ioMsg.errs:
		if err != nil {
			s.logger.Error("get error result from write", slog.String("err", err.Error()))
		}
		return err
	case <-ctx.Done():
		s.logger.Error("failed to wait for write result", slog.String("err", ctx.Err().Error()))
		return ctx.Err()
	case <-s.done:
		s.logger.Warn("session is closed while waiting for write result", slog.String("message", string(msgBs)))
		return nil
	}
}

func (s stdIOSession) Messages() iter.Seq[JSONRPCMessage] {
	return func(yield func(JSONRPCMessage) bool) {
		defer close(s.readClosed)

		// Use bufio.Reader instead of bufio.Scanner to avoid max token size errors.
		reader := bufio.NewReader(s.reader)
		for {
			type lineWithErr struct {
				line string
				err  error
			}

			lines := make(chan lineWithErr)

			// We use goroutines to avoid blocking on slow readers, so we can listen
			// to done channel and return if needed.
			go func() {
				line, err := reader.ReadString('\n')
				if err != nil {
					select {
					case lines <- lineWithErr{err: err}:
					default:
					}
					return
				}
				select {
				case lines <- lineWithErr{line: strings.TrimSuffix(line, "\n")}:
				default:
				}
			}()

			var lwe lineWithErr
			select {
			case <-s.done:
				return
			case lwe = <-lines:
			}

			if lwe.err != nil {
				if errors.Is(lwe.err, io.EOF) {
					return
				}
				s.logger.Error("failed to read message", "err", lwe.err)
				return
			}

			if lwe.line == "" {
				continue
			}

			var msg JSONRPCMessage
			if err := json.Unmarshal([]byte(lwe.line), &msg); err != nil {
				s.logger.Error("failed to unmarshal message", "err", err)
				continue
			}

			// We stop iteration if yield returns false
			if !yield(msg) {
				return
			}
		}
	}
}

func (s stdIOSession) Stop() {
	close(s.done)
	<-s.readClosed
	<-s.writeClosed
}

func (s stdIOSession) processWriteMessages() {
	defer close(s.writeClosed)

	for {
		// Process writing the message queue until the session is closed.
		var msg stdIOMessage
		select {
		case <-s.done:
			return
		case msg = <-s.writeMessages:
		}

		_, err := s.writer.Write(msg.msg)

		msg.errs <- err
	}
}
