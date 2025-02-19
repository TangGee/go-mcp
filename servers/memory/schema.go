package memory

type createEntitiesArgs struct {
	Entities []entity `json:"entities"`
}

type entity struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
}

type createRelationsArgs struct {
	Relations []relation `json:"relations"`
}

type relation struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relationType"`
}

type addObservationsArgs struct {
	Observations []observation `json:"observations"`
}

type observation struct {
	EntityName string   `json:"entityName"`
	Contents   []string `json:"contents"`

	Observations []string `json:"observations,omitempty"` // For deletions.
}

type deleteEntitiesArgs struct {
	EntityNames []string `json:"entityNames"`
}

type deleteObservationsArgs struct {
	Deletions []observation `json:"deletions"`
}

type deleteRelationsArgs struct {
	Relations []relation `json:"relations"`
}

type searchNodesArgs struct {
	Query string `json:"query"`
}

type openNodesArgs struct {
	Names []string `json:"names"`
}

var createEntitiesSchema = []byte(`{
  "type": "object",
  "properties": {
    "entities": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string", "description": "The name of the entity" },
          "entityType": { "type": "string", "description": "The type of the entity" },
          "observations": { 
            "type": "array", 
            "items": { "type": "string" },
            "description": "An array of observation contents associated with the entity"
          }
        },
        "required": ["name", "entityType", "observations"]
      }
    }
  },
  "required": ["entities"]
}`)

var createRelationsSchema = []byte(`{
  "type": "object",
  "properties": {
    "relations": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "from": { "type": "string", "description": "The name of the entity where the relation starts" },
          "to": { "type": "string", "description": "The name of the entity where the relation ends" },
          "relationType": { "type": "string", "description": "The type of the relation" }
        },
        "required": ["from", "to", "relationType"]
      }
    }
  },
  "required": ["relations"]
}`)

var addObservationsSchema = []byte(`{
  "type": "object",
  "properties": {
    "observations": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "entityName": { "type": "string", "description": "The name of the entity to add the observations to" },
          "contents": { 
            "type": "array", 
            "items": { "type": "string" },
            "description": "An array of observation contents to add"
          }
        },
        "required": ["entityName", "contents"]
      }
    }
  },
  "required": ["observations"]
}`)

var deleteEntitiesSchema = []byte(`{
  "type": "object",
  "properties": {
    "entityNames": { 
      "type": "array", 
      "items": { "type": "string" },
      "description": "An array of entity names to delete" 
    }
  },
  "required": ["entityNames"]
}`)

var deleteObservationsSchema = []byte(`{
  "type": "object",
  "properties": {
    "deletions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "entityName": { "type": "string", "description": "The name of the entity containing the observations" },
          "observations": { 
            "type": "array", 
            "items": { "type": "string" },
            "description": "An array of observations to delete"
          }
        },
        "required": ["entityName", "observations"]
      }
    }
  },
  "required": ["deletions"]
}`)

var deleteRelationsSchema = []byte(`{
  "type": "object",
  "properties": {
    "relations": { 
      "type": "array", 
      "items": {
        "type": "object",
        "properties": {
          "from": { "type": "string", "description": "The name of the entity where the relation starts" },
          "to": { "type": "string", "description": "The name of the entity where the relation ends" },
          "relationType": { "type": "string", "description": "The type of the relation" }
        },
        "required": ["from", "to", "relationType"]
      },
      "description": "An array of relations to delete" 
    }
  },
  "required": ["relations"]
}`)

var readGraphSchema = []byte(`{
  "type": "object",
  "properties": {},
  "required": []
}`)

var searchNodesSchema = []byte(`{
  "type": "object",
  "properties": {
    "query": { 
      "type": "string", 
      "description": "The search query to match against entity names, types, and observation content" 
    }
  },
  "required": ["query"]
}`)

var openNodesSchema = []byte(`{
  "type": "object",
  "properties": {
    "names": {
      "type": "array",
      "items": { "type": "string" },
      "description": "An array of entity names to retrieve"
    }
  },
  "required": ["names"]
}`)
