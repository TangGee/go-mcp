package filesystem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TangGee/go-mcp"
)

func TestReadFile(t *testing.T) {
	tempDir := createTempDir(t)
	defer cleanup(t, tempDir)

	// Create a test file
	testContent := "test content"
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte(testContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test successful read
	args, _ := json.Marshal(ReadFileArgs{Path: testFile})
	result, err := readFile([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if len(result.Content) != 1 {
		t.Errorf("Expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Text != testContent {
		t.Errorf("Expected content '%s', got '%s'", testContent, result.Content[0].Text)
	}

	// Test reading non-existent file
	args, _ = json.Marshal(ReadFileArgs{Path: "nonexistent.txt"})
	_, err = readFile([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err == nil {
		t.Error("Expected error for non-existent file, got none")
	}
}

func TestWriteFile(t *testing.T) {
	tempDir := createTempDir(t)
	defer cleanup(t, tempDir)

	testContent := "test content"
	testFile := filepath.Join(tempDir, "write_test.txt")

	args, _ := json.Marshal(WriteFileArgs{
		Path:    testFile,
		Content: testContent,
	})

	_, err := writeFile([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify file contents
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Errorf("Failed to read written file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("Expected content '%s', got '%s'", testContent, string(content))
	}
}

func TestListDirectory(t *testing.T) {
	tempDir := createTempDir(t)
	defer cleanup(t, tempDir)

	// Create test files and directories
	testFiles := []string{"file1.txt", "file2.txt"}
	testDirs := []string{"dir1", "dir2"}

	for _, file := range testFiles {
		err := os.WriteFile(filepath.Join(tempDir, file), []byte("test"), 0600)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	for _, dir := range testDirs {
		err := os.Mkdir(filepath.Join(tempDir, dir), 0700)
		if err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}
	}

	args, _ := json.Marshal(ListDirectoryArgs{Path: tempDir})
	result, err := listDirectory([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(result.Content) != len(testFiles)+len(testDirs) {
		t.Errorf("Expected %d items, got %d", len(testFiles)+len(testDirs), len(result.Content))
	}

	// Verify files and directories are listed correctly
	for _, content := range result.Content {
		if !strings.HasPrefix(content.Text, "[FILE] ") && !strings.HasPrefix(content.Text, "[DIR] ") {
			t.Errorf("Invalid content format: %s", content.Text)
		}
	}
}

func TestReadMultipleFiles(t *testing.T) {
	tempDir := createTempDir(t)
	defer cleanup(t, tempDir)

	// Create test files
	files := map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
	}

	var paths []string
	for name, content := range files {
		path := filepath.Join(tempDir, name)
		err := os.WriteFile(path, []byte(content), 0600)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		paths = append(paths, path)
	}

	args, _ := json.Marshal(ReadMultipleFilesArgs{Paths: paths})
	result, err := readMultipleFiles([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if len(result.Content) != len(files) {
		t.Errorf("Expected %d contents, got %d", len(files), len(result.Content))
	}
}

func TestEditFile(t *testing.T) {
	tempDir := createTempDir(t)
	defer cleanup(t, tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "edit_test.txt")
	initialContent := "line1\nline2\nline3\n"
	err := os.WriteFile(testFile, []byte(initialContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	edits := []EditOperation{
		{
			OldText: "line2",
			NewText: "modified line2",
		},
	}

	args, _ := json.Marshal(EditFileArgs{
		Path:   testFile,
		Edits:  edits,
		DryRun: false,
	})

	_, err = editFile([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify file was modified
	content, _ := os.ReadFile(testFile)
	if !strings.Contains(string(content), "modified line2") {
		t.Error("File content was not modified as expected")
	}
}

func TestCreateDirectory(t *testing.T) {
	tempDir := createTempDir(t)
	defer cleanup(t, tempDir)

	newDir := filepath.Join(tempDir, "new_dir", "nested_dir")
	args, _ := json.Marshal(CreateDirectoryArgs{Path: newDir})

	_, err := createDirectory([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify directory was created
	if info, err := os.Stat(newDir); err != nil || !info.IsDir() {
		t.Error("Directory was not created as expected")
	}
}

func TestDirectoryTree(t *testing.T) {
	tempDir := createTempDir(t)
	defer cleanup(t, tempDir)

	// Create test structure
	err := os.MkdirAll(filepath.Join(tempDir, "dir1", "subdir"), 0700)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "dir1", "file1.txt"), []byte("test"), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	args, _ := json.Marshal(DirectoryTreeArgs{Path: tempDir})
	result, err := directoryTree([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify JSON structure
	var treeData []interface{}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &treeData); err != nil {
		t.Errorf("Invalid JSON structure: %v", err)
	}

	// Verify the structure contains expected data
	if len(treeData) == 0 {
		t.Error("Tree data is empty")
	}
}

func TestMoveFile(t *testing.T) {
	tempDir := createTempDir(t)
	defer cleanup(t, tempDir)

	// Create source file
	sourcePath := filepath.Join(tempDir, "source.txt")
	destPath := filepath.Join(tempDir, "dest.txt")
	err := os.WriteFile(sourcePath, []byte("test content"), 0600)
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	args, _ := json.Marshal(MoveFileArgs{
		Source:      sourcePath,
		Destination: destPath,
	})

	_, err = moveFile([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify file was moved
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Error("Source file still exists")
	}
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("Destination file doesn't exist")
	}
}

func TestSearchFiles(t *testing.T) {
	tempDir := createTempDir(t)
	defer cleanup(t, tempDir)

	// Create test files
	testFiles := []string{"test1.txt", "test2.txt", "other.txt"}
	for _, file := range testFiles {
		if err := os.WriteFile(filepath.Join(tempDir, file), []byte("test"), 0600); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	args, _ := json.Marshal(SearchFilesArgs{
		Path:    tempDir,
		Pattern: "test*.txt",
	})

	result, err := searchFiles([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Should find 2 files
	if len(result.Content) != 2 {
		t.Errorf("Expected 2 files, got %d", len(result.Content))
	}
}

func TestGetFileInfo(t *testing.T) {
	tempDir := createTempDir(t)
	defer cleanup(t, tempDir)

	testFile := filepath.Join(tempDir, "info_test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	args, _ := json.Marshal(GetFileInfoArgs{Path: testFile})
	result, err := getFileInfo([]string{tempDir}, mcp.CallToolParams{Arguments: args})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify JSON structure
	var fileInfo struct {
		Size        int64       `json:"size"`
		IsDirectory bool        `json:"isDirectory"`
		IsFile      bool        `json:"isFile"`
		Permissions os.FileMode `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &fileInfo); err != nil {
		t.Errorf("Invalid JSON structure: %v", err)
	}

	if fileInfo.Size != int64(len("test content")) {
		t.Errorf("Incorrect file size")
	}
	if fileInfo.IsDirectory {
		t.Error("File incorrectly marked as directory")
	}
	if !fileInfo.IsFile {
		t.Error("Not marked as regular file")
	}
}

func TestListAllowedDirectories(t *testing.T) {
	rootPaths := []string{"/path1", "/path2"}
	result, err := listAllowedDirectories(rootPaths, mcp.CallToolParams{})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if len(result.Content) != len(rootPaths) {
		t.Errorf("Expected %d paths, got %d", len(rootPaths), len(result.Content))
	}
	for i, content := range result.Content {
		if content.Text != rootPaths[i] {
			t.Errorf("Expected path %s, got %s", rootPaths[i], content.Text)
		}
	}
}

func createTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "filesystem_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return dir
}

func cleanup(t *testing.T, path string) {
	err := os.RemoveAll(path)
	if err != nil {
		t.Errorf("Failed to cleanup: %v", err)
	}
}
