package filesystem

// ReadFileArgs is an argument struct for the read_file tool.
type ReadFileArgs struct {
	Path string `json:"path"`
}

// ReadMultipleFilesArgs is an argument struct for the read_multiple_files tool.
type ReadMultipleFilesArgs struct {
	Paths []string `json:"paths"`
}

// WriteFileArgs is an argument struct for the write_file tool.
type WriteFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// EditFileArgs is an argument struct for the edit_file tool.
type EditFileArgs struct {
	Path   string          `json:"path"`
	Edits  []EditOperation `json:"edits"`
	DryRun bool            `json:"dryRun"`
}

// EditOperation is a struct representing an edit operation.
type EditOperation struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

// CreateDirectoryArgs is an argument struct for the create_directory tool.
type CreateDirectoryArgs struct {
	Path string `json:"path"`
}

// ListDirectoryArgs is an argument struct for the list_directory tool.
type ListDirectoryArgs struct {
	Path string `json:"path"`
}

// DirectoryTreeArgs is an argument struct for the directory_tree tool.
type DirectoryTreeArgs struct {
	Path string `json:"path"`
}

// MoveFileArgs is an argument struct for the move_file tool.
type MoveFileArgs struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// SearchFilesArgs is an argument struct for the search_files tool.
type SearchFilesArgs struct {
	Path    string   `json:"path"`
	Pattern string   `json:"pattern"`
	Exclude []string `json:"excludePatterns"`
}

// GetFileInfoArgs is an argument struct for the get_file_info tool.
type GetFileInfoArgs struct {
	Path string `json:"path"`
}

var readFileSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "path": {
        "type": "string"
      }
    },
    "required": ["path"]
  }
`)

var readMultipleFilesSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "paths": {
        "type": "array",
        "items": {
          "type": "string"
        }
      }
    },
    "required": ["paths"]
  }
`)

var writeFileSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "path": {
        "type": "string"
      },
      "content": {
        "type": "string"
      }
    },
    "required": ["path", "content"]
  }
`)

var editFileSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "path": {
        "type": "string"
      },
      "edits": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "oldText": {
              "type": "string"
            },
            "newText": {
              "type": "string"
            }
          },
          "required": ["oldText", "newText"]
        }
      },
      "dryRun": {
        "type": "boolean"
      }
    },
    "required": ["path", "edits"]
  }
`)

var createDirectorySchema = []byte(`
  {
    "type": "object",
    "properties": {
      "path": {
        "type": "string"
      }
    },
    "required": ["path"]
  }
`)

var listDirectorySchema = []byte(`
  {
    "type": "object",
    "properties": {
      "path": {
        "type": "string"
      }
    },
    "required": ["path"]
  }
`)

var directoryTreeSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "path": {
        "type": "string"
      }
    },
    "required": ["path"]
  }
`)

var moveFileSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "source": {
        "type": "string"
      },
      "destination": {
        "type": "string"
      }
    },
    "required": ["source", "destination"]
  }
`)

var searchFilesSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "path": {
        "type": "string"
      },
      "pattern": {
        "type": "string"
      },
      "excludePatterns": {
        "type": "array",
        "items": {
          "type": "string"
        }
      }
    },
    "required": ["path", "pattern"]
  }
`)

var getFileInfoSchema = []byte(`
  {
    "type": "object",
    "properties": {
      "path": {
        "type": "string"
      }
    },
    "required": ["path"]
  }
`)
