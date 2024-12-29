package everything

import "github.com/MegaGrindStone/go-mcp"

// LogStreams implements mcp.LogHandler interface.
func (s *Server) LogStreams() <-chan mcp.LogParams {
	return s.logChan
}

// SetLogLevel implements mcp.LogHandler interface.
func (s *Server) SetLogLevel(level mcp.LogLevel) {
	s.logLevel = level
}

func (s *Server) log(msg string, level mcp.LogLevel) {
	if level < s.logLevel {
		return
	}

	select {
	case s.logChan <- mcp.LogParams{
		Level:  level,
		Logger: "everything",
		Data: mcp.LogData{
			Message: msg,
		},
	}:
	case <-s.doneChan:
		return
	}
}
