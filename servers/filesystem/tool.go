package filesystem

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/TangGee/go-mcp"
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
		{
			Name:        "list_allowed_directories",
			Description: ``,
			InputSchema: listAllowedDirectoriesSchema,
		},
	},
}

func readFile(rootPaths []string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var rfParams ReadFileArgs
	if err := json.Unmarshal(params.Arguments, &rfParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	validPath, err := validatePath(rfParams.Path, rootPaths)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	bs, err := os.ReadFile(validPath)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to read file with path %s: %w", validPath, err)
	}

	if len(bs) == 0 {
		return mcp.CallToolResult{}, fmt.Errorf("file with path %s is empty", validPath)
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

func readMultipleFiles(rootPaths []string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var rmfParams ReadMultipleFilesArgs
	if err := json.Unmarshal(params.Arguments, &rmfParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	resultChan := make(chan mcp.Content, len(rmfParams.Paths))
	var wg sync.WaitGroup

	for _, path := range rmfParams.Paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			validPath, err := validatePath(p, rootPaths)
			if err != nil {
				resultChan <- mcp.Content{
					Type: mcp.ContentTypeText,
					Text: fmt.Sprintf("failed to validate path %s: %s", p, err),
				}
				return
			}

			bs, err := os.ReadFile(validPath)
			if err != nil {
				resultChan <- mcp.Content{
					Type: mcp.ContentTypeText,
					Text: fmt.Sprintf("failed to read file with path %s: %s", p, err),
				}
				return
			}

			content := fmt.Sprintf("File content of %s:\n%s\n", p, string(bs))
			resultChan <- mcp.Content{
				Type: mcp.ContentTypeText,
				Text: content,
			}
		}(path)
	}

	// Wait in a separate goroutine and close the channel when done
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	var result []mcp.Content
	for content := range resultChan {
		result = append(result, content)
	}

	return mcp.CallToolResult{
		Content: result,
		IsError: false,
	}, nil
}

func writeFile(rootPaths []string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var wfParams WriteFileArgs
	if err := json.Unmarshal(params.Arguments, &wfParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	validPath, err := validatePath(wfParams.Path, rootPaths)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	if err = os.WriteFile(validPath, []byte(wfParams.Content), 0600); err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to write file with path %s: %w", validPath, err)
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

func editFile(rootPaths []string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var efParams EditFileArgs
	if err := json.Unmarshal(params.Arguments, &efParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	validPath, err := validatePath(efParams.Path, rootPaths)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	result, err := applyFileEdits(validPath, efParams.Edits, efParams.DryRun)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: result,
			},
		},
		IsError: false,
	}, nil
}

func createDirectory(rootPaths []string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var cdParams CreateDirectoryArgs
	if err := json.Unmarshal(params.Arguments, &cdParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	validPath, err := validatePath(cdParams.Path, rootPaths)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	if err := os.MkdirAll(validPath, 0700); err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to create directory with path %s: %w", validPath, err)
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

func listDirectory(rootPaths []string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var ldParams ListDirectoryArgs
	if err := json.Unmarshal(params.Arguments, &ldParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	validPath, err := validatePath(ldParams.Path, rootPaths)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	files, err := os.ReadDir(validPath)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to read directory with path %s: %w", validPath, err)
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

func directoryTree(rootPaths []string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var dtParams DirectoryTreeArgs
	if err := json.Unmarshal(params.Arguments, &dtParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	result, err := buildTree(rootPaths, dtParams.Path)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	resultJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(resultJSON),
			},
		},
		IsError: false,
	}, nil
}

func moveFile(rootPaths []string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var mfParams MoveFileArgs
	if err := json.Unmarshal(params.Arguments, &mfParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	validSourcePath, err := validatePath(mfParams.Source, rootPaths)
	if err != nil {
		return mcp.CallToolResult{}, err
	}
	validDestinationPath, err := validatePath(mfParams.Destination, rootPaths)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	if err := os.Rename(validSourcePath, validDestinationPath); err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to move file with path %s: %w", validSourcePath, err)
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

func searchFiles(rootPaths []string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var sfParams SearchFilesArgs
	if err := json.Unmarshal(params.Arguments, &sfParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	results, err := searchFilesWithPattern(sfParams.Path, sfParams.Pattern, rootPaths, sfParams.Exclude)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	if len(results) == 0 {
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

	var result []mcp.Content
	for _, path := range results {
		result = append(result, mcp.Content{
			Type: mcp.ContentTypeText,
			Text: path,
		})
	}

	return mcp.CallToolResult{
		Content: result,
		IsError: false,
	}, nil
}

func getFileInfo(rootPaths []string, params mcp.CallToolParams) (mcp.CallToolResult, error) {
	var gfiParams GetFileInfoArgs
	if err := json.Unmarshal(params.Arguments, &gfiParams); err != nil {
		return mcp.CallToolResult{}, err
	}

	validPath, err := validatePath(gfiParams.Path, rootPaths)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	info, err := os.Stat(validPath)
	if err != nil {
		return mcp.CallToolResult{}, fmt.Errorf("failed to stat file with path %s: %w", validPath, err)
	}

	type fileStat struct {
		Size        int64       `json:"size"`
		Created     time.Time   `json:"created"`
		Modified    time.Time   `json:"modified"`
		Accessed    time.Time   `json:"accessed"`
		IsDirectory bool        `json:"isDirectory"`
		IsFile      bool        `json:"isFile"`
		Permissions os.FileMode `json:"permissions"`
	}

	st := fileStat{
		Size:        info.Size(),
		Created:     info.ModTime(),
		Modified:    info.ModTime(),
		Accessed:    info.ModTime(),
		IsDirectory: info.IsDir(),
		IsFile:      info.Mode().IsRegular(),
		Permissions: info.Mode(),
	}

	resultJSON, err := json.Marshal(st)
	if err != nil {
		return mcp.CallToolResult{}, err
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: mcp.ContentTypeText,
				Text: string(resultJSON),
			},
		},
		IsError: false,
	}, nil
}

func listAllowedDirectories(rootPaths []string, _ mcp.CallToolParams) (mcp.CallToolResult, error) {
	var result []mcp.Content

	for _, rootPath := range rootPaths {
		result = append(result, mcp.Content{
			Type: mcp.ContentTypeText,
			Text: rootPath,
		})
	}

	return mcp.CallToolResult{
		Content: result,
		IsError: false,
	}, nil
}
