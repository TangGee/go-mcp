package everything

import (
	"encoding/json"
	"iter"

	"github.com/TangGee/go-mcp"
)

// LogStreams implements mcp.LogHandler interface.
func (s *Server) LogStreams() iter.Seq[mcp.LogParams] {
	defer close(s.logClosed)
	return func(yield func(mcp.LogParams) bool) {
		for {
			select {
			case <-s.done:
				return
			case params := <-s.logs:
				if !yield(params) {
					return
				}
			}
		}
	}
}

// SetLogLevel implements mcp.LogHandler interface.
func (s *Server) SetLogLevel(level mcp.LogLevel) {
	s.logLevel = level
}

func (s *Server) log(msg string, level mcp.LogLevel) {
	if level < s.logLevel {
		return
	}

	type logData struct {
		Message string `json:"message"`
	}
	data := logData{
		Message: msg,
	}
	dataBs, _ := json.Marshal(data)

	select {
	case s.logs <- mcp.LogParams{
		Level:  level,
		Logger: "everything",
		Data:   dataBs,
	}:
	case <-s.done:
		return
	}
}
