package everything

import "github.com/MegaGrindStone/go-mcp/pkg/mcp"

// ProgressReports implements mcp.ProgressReporter interface.
func (s *Server) ProgressReports() <-chan mcp.ProgressParams {
	return s.progressChan
}
