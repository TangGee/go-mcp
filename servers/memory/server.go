package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TangGee/go-mcp"
)

// Server implements the Model Context Protocol (MCP) for memory operations. It provides
// memory-based operations through a restricted knowledge base, exposing standard
// memory operations as MCP tools.
//
// Server ensures all operations remain within the configured knowledge base for security.
// It implements both the mcp.Server and mcp.ToolServer interfaces to provide memory
// functionality through the MCP protocol.
type Server struct {
	kb knowledgeBase
}

// NewServer creates a new memory MCP server that provides access to the knowledge base
// at the specified memoryFilePath.
//
// The server validates that the knowledge base exists and is a valid JSON file. All memory
// operations are restricted to this knowledge base and its contents for security.
func NewServer(memoryFilePath string) Server {
	return Server{
		kb: newKnowledgeBase(memoryFilePath),
	}
}

// ListTools implements mcp.ToolServer interface.
// Returns the list of available memory tools supported by this server.
// The tools provide various memory operations like creating, deleting, and searching
// entities and relations. This tool is essential for understanding memory structure
// and finding specific entities and relations. Only works within allowed knowledge bases.
//
// Returns a ToolList containing all available memory tools and any error encountered.
func (s Server) ListTools(
	context.Context,
	mcp.ListToolsParams,
	mcp.ProgressReporter,
	mcp.RequestClientFunc,
) (mcp.ListToolsResult, error) {
	return toolList, nil
}

// CallTool implements mcp.ToolServer interface.
// Executes a specified memory tool with the given parameters.
// All operations are restricted to the knowledge base.
//
// Returns the tool's execution result and any error encountered.
// Returns error if the tool is not found or if execution fails.
func (s Server) CallTool(
	_ context.Context,
	params mcp.CallToolParams,
	_ mcp.ProgressReporter,
	_ mcp.RequestClientFunc,
) (mcp.CallToolResult, error) {
	switch params.Name {
	case "create_entities":
		return s.createEntities(params)
	case "create_relations":
		return s.createRelations(params)
	case "add_observations":
		return s.addObservations(params)
	case "delete_entities":
		return s.deleteEntities(params)
	case "delete_observations":
		return s.deleteObservations(params)
	case "delete_relations":
		return s.deleteRelations(params)
	case "read_graph":
		return s.readGraph()
	case "search_nodes":
		return s.searchNodes(params)
	case "open_nodes":
		return s.openNodes(params)
	default:
		return mcp.CallToolResult{}, fmt.Errorf("tool not found: %s", params.Name)
	}
}

func (s Server) createEntities(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var ceParams createEntitiesArgs
	if err := json.Unmarshal(params.Arguments, &ceParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	entities, err := s.kb.createEntities(ceParams.Entities)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	entitiesJSON, err := json.Marshal(entities)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(entitiesJSON),
			},
		},
		IsError: false,
	}, nil
}

func (s Server) createRelations(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var crParams createRelationsArgs
	if err := json.Unmarshal(params.Arguments, &crParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	relations, err := s.kb.createRelations(crParams.Relations)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	relationsJSON, err := json.Marshal(relations)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(relationsJSON),
			},
		},
		IsError: false,
	}, nil
}

func (s Server) addObservations(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var aoParams addObservationsArgs
	if err := json.Unmarshal(params.Arguments, &aoParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	observations, err := s.kb.addObservations(aoParams.Observations)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	observationsJSON, err := json.Marshal(observations)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(observationsJSON),
			},
		},
		IsError: false,
	}, nil
}

func (s Server) deleteEntities(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var deParams deleteEntitiesArgs
	if err := json.Unmarshal(params.Arguments, &deParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	err := s.kb.deleteEntities(deParams.EntityNames)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: "Entities deleted successfully",
			},
		},
		IsError: false,
	}, nil
}

func (s Server) deleteObservations(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var doParams deleteObservationsArgs
	if err := json.Unmarshal(params.Arguments, &doParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	err := s.kb.deleteObservations(doParams.Deletions)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: "Observations deleted successfully",
			},
		},
		IsError: false,
	}, nil
}

func (s Server) deleteRelations(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var drParams deleteRelationsArgs
	if err := json.Unmarshal(params.Arguments, &drParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	err := s.kb.deleteRelations(drParams.Relations)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: "Relations deleted successfully",
			},
		},
		IsError: false,
	}, nil
}

func (s Server) readGraph() (mcp.CallToolResult, error) {
	graph, err := s.kb.readGraph()
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	graphJSON, err := json.Marshal(graph)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(graphJSON),
			},
		},
		IsError: false,
	}, nil
}

func (s Server) searchNodes(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var snParams searchNodesArgs
	if err := json.Unmarshal(params.Arguments, &snParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	graph, err := s.kb.searchNodes(snParams.Query)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	graphJSON, err := json.Marshal(graph)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(graphJSON),
			},
		},
		IsError: false,
	}, nil
}

func (s Server) openNodes(params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var onParams openNodesArgs
	if err := json.Unmarshal(params.Arguments, &onParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	graph, err := s.kb.openNodes(onParams.Names)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	graphJSON, err := json.Marshal(graph)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(graphJSON),
			},
		},
		IsError: false,
	}, nil
}
