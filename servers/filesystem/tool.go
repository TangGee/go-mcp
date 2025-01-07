package filesystem

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/MegaGrindStone/go-mcp"
)

var toolList = mcp.ListToolsResult{
	Tools: []mcp.Tool{
		{
			Name: "read_file",
			Description: `
Read the complete contents of a file from the file system.
Handles various text encodings and provides detailed error messages
if the file cannot be read. Use this tool when you need to examine
the contents of a single file. Only works within allowed directories.,
        `,
			InputSchema: readFileSchema,
		},
		{
			Name: "read_multiple_files",
			Description: `
Read the contents of multiple files simultaneously. This is more
efficient than reading files one by one when you need to analyze
or compare multiple files. Each file's content is returned with its
path as a reference. Failed reads for individual files won't stop
the entire operation. Only works within allowed directories.
        `,
			InputSchema: readMultipleFilesSchema,
		},
		{
			Name: "write_file",
			Description: `
Create a new file or completely overwrite an existing file with new content.
Use with caution as it will overwrite existing files without warning.
Handles text content with proper encoding. Only works within allowed directories.
        `,
			InputSchema: writeFileSchema,
		},
		{
			Name: "edit_file",
			Description: `
Make line-based edits to a text file. Each edit replaces exact line sequences
with new content. Returns a git-style diff showing the changes made.
Only works within allowed directories.
        `,
			InputSchema: editFileSchema,
		},
		{
			Name: "create_directory",
			Description: `
Create a new directory or ensure a directory exists. Can create multiple
nested directories in one operation. If the directory already exists,
this operation will succeed silently. Perfect for setting up directory
structures for projects or ensuring required paths exist. Only works within allowed directories.
        `,
			InputSchema: createDirectorySchema,
		},
		{
			Name: "list_directory",
			Description: `
Get a detailed listing of all files and directories in a specified path.
Results clearly distinguish between files and directories with [FILE] and [DIR]
prefixes. This tool is essential for understanding directory structure and
finding specific files within a directory. Only works within allowed directories.
        `,
			InputSchema: listDirectorySchema,
		},
		{
			Name: "directory_tree",
			Description: `
Get a recursive tree view of files and directories as a JSON structure.
Each entry includes 'name', 'type' (file/directory), and 'children' for directories.
Files have no children array, while directories always have a children array (which may be empty).
The output is formatted with 2-space indentation for readability. Only works within allowed directories.
        `,
			InputSchema: directoryTreeSchema,
		},
		{
			Name: "move_file",
			Description: `Move or rename files and directories. Can move files between directories
and rename them in a single operation. If the destination exists, the
operation will fail. Works across different directories and can be used
for simple renaming within the same directory. Both source and destination must be within allowed directories.
        `,
			InputSchema: moveFileSchema,
		},
		{
			Name: "search_files",
			Description: `Recursively search for files and directories matching a pattern.
Searches through all subdirectories from the starting path. The search
is case-insensitive and matches partial names. Returns full paths to all
matching items. Great for finding files when you don't know their exact location.
Only searches within allowed directories.
        `,
			InputSchema: searchFilesSchema,
		},
		{
			Name: "get_file_info",
			Description: `Retrieve detailed metadata about a file or directory. Returns comprehensive
information including size, creation time, last modified time, permissions,
and type. This tool is perfect for understanding file characteristics
without reading the actual content. Only works within allowed directories.
        `,
			InputSchema: getFileInfoSchema,
		},
	},
}

func readFile(rootPath string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var rfParams ReadFileArgs
	if err := json.Unmarshal(params.Arguments, &rfParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	fullPath := filepath.Join(rootPath, filepath.Clean(rfParams.Path))

	info, err := os.Stat(fullPath)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to stat file with path %s: %w", fullPath, err)
	}

	if info.IsDir() {
		return mcp.CallToolResult{}, fmt.Errorf("path %s is a directory, not a file", fullPath)
	}

	bs, err := os.ReadFile(fullPath)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to read file with path %s: %w", fullPath, err)
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(bs),
			},
		},
		IsError: false,
	}, nil
}

func readMultipleFiles(rootPath string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var rmfParams ReadMultipleFilesArgs
	if err := json.Unmarshal(params.Arguments, &rmfParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	var result []mcp.Content

	for _, path := range rmfParams.Paths {
		fullPath := filepath.Join(rootPath, filepath.Clean(path))

		info, err := os.Stat(fullPath)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to stat file with path %s: %w", fullPath, err)
		}

		if info.IsDir() {
			return mcp.CallToolResult{}, fmt.Errorf("path %s is a directory, not a file", fullPath)
		}

		bs, err := os.ReadFile(fullPath)
		if err != nil {
			return mcp.CallToolResult{}, fmt.Errorf("failed to read file with path %s: %w", fullPath, err)
		}

		content := fmt.Sprintf("File content of %s:\n%s\n", path, string(bs))

		result = append(result, mcp.Content{
			Type: mcp.ContentTypeText,
			Text: content,
		})
	}

	return mcp.CallToolResult{
		Content: result,
		IsError: false,
	}, nil
}

func writeFile(rootPath string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var wfParams WriteFileArgs
	if err := json.Unmarshal(params.Arguments, &wfParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	fullPath := filepath.Join(rootPath, filepath.Clean(wfParams.Path))

	err := os.WriteFile(fullPath, []byte(wfParams.Content), 0600)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to write file with path %s: %w", fullPath, err)
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("File %s written successfully", wfParams.Path),
			},
		},
		IsError: false,
	}, nil
}

func editFile(rootPath string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var efParams EditFileArgs
	if err := json.Unmarshal(params.Arguments, &efParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	fullPath := filepath.Join(rootPath, filepath.Clean(efParams.Path))

	bs, err := os.ReadFile(fullPath)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to read file with path %s: %w", fullPath, err)
	}

	newContent := string(bs)

	// TODO: Use more sophisticated diff algorithm to match the typescript implementation.

	for _, edit := range efParams.Edits {
		newContent = strings.ReplaceAll(newContent, edit.OldText, edit.NewText)
	}

	if efParams.DryRun {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: mcp.ContentTypeText,
					Text: fmt.Sprintf("File %s edited successfully", efParams.Path),
				},
			},
			IsError: false,
		}, nil
	}

	err = os.WriteFile(fullPath, []byte(newContent), 0600)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to write file with path %s: %w", fullPath, err)
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("File %s edited successfully", efParams.Path),
			},
		},
		IsError: false,
	}, nil
}

func createDirectory(rootPath string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var cdParams CreateDirectoryArgs
	if err := json.Unmarshal(params.Arguments, &cdParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	fullPath := filepath.Join(rootPath, filepath.Clean(cdParams.Path))

	err := os.MkdirAll(fullPath, 0700)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to create directory with path %s: %w", fullPath, err)
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("Directory %s created successfully", cdParams.Path),
			},
		},
	}, nil
}

func listDirectory(rootPath string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var ldParams ListDirectoryArgs
	if err := json.Unmarshal(params.Arguments, &ldParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	fullPath := filepath.Join(rootPath, filepath.Clean(ldParams.Path))

	files, err := os.ReadDir(fullPath)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to read directory with path %s: %w", fullPath, err)
	}

	var result []mcp.Content

	for _, file := range files {
		prefix := "[FILE] "
		if file.IsDir() {
			prefix = "[DIR] "
		}

		content := fmt.Sprintf("%s%s\n", prefix, file.Name())

		result = append(result, mcp.Content{
			Type: mcp.ContentTypeText,
			Text: content,
		})
	}

	return mcp.CallToolResult{
		Content: result,
		IsError: false,
	}, nil
}

func directoryTree(rootPath string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var dtParams DirectoryTreeArgs
	if err := json.Unmarshal(params.Arguments, &dtParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	fullPath := filepath.Join(rootPath, filepath.Clean(dtParams.Path))

	files, err := os.ReadDir(fullPath)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to read directory with path %s: %w", fullPath, err)
	}

	// TODO: Display actual directory tree instead of just directory listing.

	var result []mcp.Content

	for _, file := range files {
		prefix := "[FILE] "
		if file.IsDir() {
			prefix = "[DIR] "
		}

		content := fmt.Sprintf("%s%s\n", prefix, file.Name())

		result = append(result, mcp.Content{
			Type: mcp.ContentTypeText,
			Text: content,
		})
	}

	return mcp.CallToolResult{
		Content: result,
		IsError: false,
	}, nil
}

func moveFile(rootPath string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var mfParams MoveFileArgs
	if err := json.Unmarshal(params.Arguments, &mfParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	fullSourcePath := filepath.Join(rootPath, filepath.Clean(mfParams.Source))
	fullDestinationPath := filepath.Join(rootPath, filepath.Clean(mfParams.Destination))

	err := os.Rename(fullSourcePath, fullDestinationPath)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to move file with path %s: %w", fullSourcePath, err)
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("File %s moved successfully", mfParams.Source),
			},
		},
		IsError: false,
	}, nil
}

func searchFiles(rootPath string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var sfParams SearchFilesArgs
	if err := json.Unmarshal(params.Arguments, &sfParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	var result []mcp.Content

	if err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		matched, err := filepath.Match(sfParams.Pattern, path)
		if err != nil {
			return err
		}

		if !matched {
			return nil
		}

		for _, ep := range sfParams.Exclude {
			exMatched, err := filepath.Match(ep, path)
			if err != nil {
				return err
			}

			if exMatched {
				return nil
			}
		}

		result = append(result, mcp.Content{
			Type: mcp.ContentTypeText,
			Text: path,
		})

		return nil
	}); err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to search files: %w", err)
	}

	if len(result) == 0 {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: mcp.ContentTypeText,
					Text: "No files found",
				},
			},
			IsError: false,
		}, nil
	}

	return mcp.CallToolResult{
		Content: result,
		IsError: false,
	}, nil
}

func getFileInfo(rootPath string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var gfiParams GetFileInfoArgs
	if err := json.Unmarshal(params.Arguments, &gfiParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	fullPath := filepath.Join(rootPath, filepath.Clean(gfiParams.Path))

	info, err := os.Stat(fullPath)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to stat file with path %s: %w", fullPath, err)
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("File %s info:\nSize: %d\nLast modified: %s\n", gfiParams.Path, info.Size(), info.ModTime()),
			},
		},
		IsError: false,
	}, nil
}
