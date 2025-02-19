package memory

import "github.com/MegaGrindStone/go-mcp"

var toolList = mcp.ListToolsResult{
	Tools: []mcp.Tool{
		{
			Name: "create_entities",
			Description: `
Create multiple new entities in the knowledge graph.
      `,
			InputSchema: createEntitiesSchema,
		},
		{
			Name: "create_relations",
			Description: `
Create multiple new relations between entities in the knowledge graph. Relations should be in active voice.
      `,
			InputSchema: createRelationsSchema,
		},
		{
			Name: "add_observations",
			Description: `
Add new observations to existing entities in the knowledge graph.
      `,
			InputSchema: addObservationsSchema,
		},
		{
			Name: "delete_entities",
			Description: `
Delete multiple entities and their associated relations from the knowledge graph.
      `,
			InputSchema: deleteEntitiesSchema,
		},
		{
			Name: "delete_observations",
			Description: `
Delete specific observations from entities in the knowledge graph.
      `,
			InputSchema: deleteObservationsSchema,
		},
		{
			Name: "delete_relations",
			Description: `
Delete multiple relations from the knowledge graph.
      `,
			InputSchema: deleteRelationsSchema,
		},
		{
			Name: "read_graph",
			Description: `
Read the entire knowledge graph.
      `,
			InputSchema: readGraphSchema,
		},
		{
			Name: "search_nodes",
			Description: `
Search for nodes in the knowledge graph based on a query.
      `,
			InputSchema: searchNodesSchema,
		},
		{
			Name: "open_nodes",
			Description: `
Open specific nodes in the knowledge graph by their names.
      `,
			InputSchema: openNodesSchema,
		},
	},
}
