package everything

import "github.com/MegaGrindStone/go-mcp"

// ProgressReports implements mcp.ProgressReporter interface.
func (s *Server) ProgressReports() <-chan mcp.ProgressParams {
	return s.progressChan
}
