package filesystem

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gobwas/glob"
	"github.com/sergi/go-diff/diffmatchpatch"
)

type treeEntry struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"` // "file" or "directory"
	Children []treeEntry `json:"children,omitempty"`
}

func validatePath(requestedPath string, allowedDirectories []string) (string, error) {
	// Expand home directory if path contains ~
	expandedPath := os.ExpandEnv(filepath.FromSlash(requestedPath))

	// Convert to absolute path
	absolute, err := filepath.Abs(expandedPath)
	if err != nil {
		return "", err
	}

	// Normalize path for consistent comparison
	normalizedRequested := filepath.Clean(absolute)

	// Check if path is within allowed directories
	isAllowed := false
	for _, dir := range allowedDirectories {
		if isSubpath(normalizedRequested, dir) {
			isAllowed = true
			break
		}
	}
	if !isAllowed {
		return "", fmt.Errorf("access denied - path %s outside allowed directories %s",
			requestedPath, strings.Join(allowedDirectories, ", "))
	}

	// Handle symlinks by checking their real path
	realPath, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}

		// For new files that don't exist yet, verify parent directory
		parentDir := filepath.Dir(absolute)
		realParentPath, err := filepath.EvalSymlinks(parentDir)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("access denied - parent directory %s does not exist", parentDir)
			}
			return "", err
		}

		// Check if parent directory is within allowed directories
		normalizedParent := filepath.Clean(realParentPath)
		isParentAllowed := false
		for _, dir := range allowedDirectories {
			if isSubpath(normalizedParent, dir) {
				isParentAllowed = true
				break
			}
		}
		if !isParentAllowed {
			return "", fmt.Errorf("access denied - parent directory %s outside allowed directories %s",
				parentDir, strings.Join(allowedDirectories, ", "))
		}

		return absolute, nil
	}

	// Check if real path is within allowed directories
	normalizedReal := filepath.Clean(realPath)
	isRealPathAllowed := false
	for _, dir := range allowedDirectories {
		if isSubpath(normalizedReal, dir) {
			isRealPathAllowed = true
			break
		}
	}
	if !isRealPathAllowed {
		return "", fmt.Errorf("access denied - real path %s outside allowed directories %s",
			realPath, strings.Join(allowedDirectories, ", "))
	}

	return realPath, nil
}

func isSubpath(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func normalizeLineEndings(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func createUnifiedDiff(originalContent, newContent, filepath string) string {
	dmp := diffmatchpatch.New()

	// Ensure consistent line endings
	normalizedOriginal := normalizeLineEndings(originalContent)
	normalizedNew := normalizeLineEndings(newContent)

	// Create diff
	diffs := dmp.DiffMain(normalizedOriginal, normalizedNew, true)
	patches := dmp.PatchMake(diffs)

	// Format as unified diff
	var diff strings.Builder
	diff.WriteString(fmt.Sprintf("--- %s (original)\n", filepath))
	diff.WriteString(fmt.Sprintf("+++ %s (modified)\n", filepath))

	for _, patch := range patches {
		diff.WriteString(dmp.PatchToText([]diffmatchpatch.Patch{patch}))
	}

	return diff.String()
}

func applyFileEdits(filePath string, edits []EditOperation, dryRun bool) (string, error) {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Apply edits
	modifiedContent, err := applyEdits(string(content), edits)
	if err != nil {
		return "", err
	}

	// Create and format diff
	diff := createUnifiedDiff(string(content), modifiedContent, filePath)
	formattedDiff := formatDiffOutput(diff)

	// Write changes if not dry run
	if !dryRun {
		if err := os.WriteFile(filePath, []byte(modifiedContent), 0600); err != nil {
			return "", fmt.Errorf("failed to write file: %w", err)
		}
	}

	return formattedDiff, nil
}

func applyEdits(content string, edits []EditOperation) (string, error) {
	modifiedContent := normalizeLineEndings(content)

	for _, edit := range edits {
		normalizedOld := normalizeLineEndings(edit.OldText)
		normalizedNew := normalizeLineEndings(edit.NewText)

		// Try exact match first
		if strings.Contains(modifiedContent, normalizedOld) {
			modifiedContent = strings.Replace(modifiedContent, normalizedOld, normalizedNew, 1)
			continue
		}

		// Try line-by-line matching
		newContent, found := tryLineByLineMatch(modifiedContent, normalizedOld, normalizedNew)
		if !found {
			return "", fmt.Errorf("could not find exact match for edit:\n%s", edit.OldText)
		}
		modifiedContent = newContent
	}

	return modifiedContent, nil
}

func tryLineByLineMatch(content, oldText, newText string) (string, bool) {
	oldLines := strings.Split(oldText, "\n")
	contentLines := strings.Split(content, "\n")

	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		potentialMatch := contentLines[i : i+len(oldLines)]
		if isMatchingBlock(potentialMatch, oldLines) {
			return replaceMatchingBlock(contentLines, i, len(oldLines), oldLines, newText), true
		}
	}

	return content, false
}

func isMatchingBlock(contentBlock, oldLines []string) bool {
	for j, oldLine := range oldLines {
		if strings.TrimSpace(oldLine) != strings.TrimSpace(contentBlock[j]) {
			return false
		}
	}
	return true
}

func replaceMatchingBlock(
	contentLines []string,
	startIdx, blockLen int,
	oldLines []string,
	newText string,
) string {
	originalIndent := getLeadingWhitespace(contentLines[startIdx])
	newLines := processNewLines(originalIndent, oldLines, strings.Split(newText, "\n"))

	result := make([]string, 0, len(contentLines)-blockLen+len(newLines))
	result = append(result, contentLines[:startIdx]...)
	result = append(result, newLines...)
	result = append(result, contentLines[startIdx+blockLen:]...)

	return strings.Join(result, "\n")
}

func processNewLines(originalIndent string, oldLines []string, newLines []string) []string {
	result := make([]string, 0, len(newLines))

	for j, line := range newLines {
		if j == 0 {
			result = append(result, originalIndent+strings.TrimLeft(line, " \t"))
			continue
		}

		if strings.TrimSpace(line) == "" {
			result = append(result, originalIndent)
			continue
		}

		oldIndent := ""
		if j < len(oldLines) {
			oldIndent = getLeadingWhitespace(oldLines[j])
		}
		newIndent := getLeadingWhitespace(line)
		relativeIndent := len(newIndent) - len(oldIndent)

		indentAmount := math.Max(0, float64(relativeIndent))
		result = append(result, originalIndent+strings.Repeat(" ", int(indentAmount))+strings.TrimLeft(line, " \t"))
	}

	return result
}

func formatDiffOutput(diff string) string {
	numBackticks := 3
	for strings.Contains(diff, strings.Repeat("`", numBackticks)) {
		numBackticks++
	}
	return fmt.Sprintf("%s\ndiff\n%s%s\n\n",
		strings.Repeat("`", numBackticks),
		diff,
		strings.Repeat("`", numBackticks))
}

func getLeadingWhitespace(s string) string {
	return strings.TrimRight(s[:len(s)-len(strings.TrimLeft(s, " \t"))], "\n\r")
}

func buildTree(rootPaths []string, currentPath string) ([]treeEntry, error) {
	// Validate path
	validPath, err := validatePath(currentPath, rootPaths)
	if err != nil {
		return nil, fmt.Errorf("path validation failed: %w", err)
	}

	// Read directory entries
	entries, err := os.ReadDir(validPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	// Build tree entries
	result := make([]treeEntry, 0, len(entries))
	for _, entry := range entries {
		// Skip .git directory
		if entry.Name() == ".git" {
			continue
		}

		entryData := treeEntry{
			Name: entry.Name(),
			Type: "file",
		}

		if entry.IsDir() {
			entryData.Type = "directory"
			subPath := filepath.Join(currentPath, entry.Name())

			// Recursively build tree for subdirectories
			children, err := buildTree(rootPaths, subPath)
			if err != nil {
				return nil, fmt.Errorf("failed to build subtree for %s: %w", subPath, err)
			}
			entryData.Children = children
		}

		result = append(result, entryData)
	}

	return result, nil
}

func searchFilesWithPattern(rootPath, pattern string, rootPaths, excludePatterns []string) ([]string, error) {
	var results []string
	var mu sync.Mutex // Mutex for thread-safe results access
	var wg sync.WaitGroup

	// Channel for limiting concurrent goroutines
	semaphore := make(chan struct{}, 50) // Limit to 50 concurrent goroutines

	// Compile exclude patterns
	var compiledPatterns []glob.Glob
	for _, pattern := range excludePatterns {
		if !strings.Contains(pattern, "*") {
			pattern = "**/" + pattern + "/**"
		}
		compiled, err := glob.Compile(pattern, '/')
		if err != nil {
			return nil, err
		}
		compiledPatterns = append(compiledPatterns, compiled)
	}

	searchPattern := strings.ToLower(pattern)

	// Define recursive search function
	var search func(currentPath string) error
	search = func(currentPath string) error {
		defer wg.Done()

		// Validate current path
		validPath, err := validatePath(currentPath, rootPaths)
		if err != nil {
			return nil
		}

		entries, err := os.ReadDir(validPath)
		if err != nil {
			return nil
		}

		for _, entry := range entries {
			fullPath := filepath.Join(currentPath, entry.Name())

			_, err := validatePath(fullPath, rootPaths)
			if err != nil {
				continue
			}

			relativePath, err := filepath.Rel(rootPath, fullPath)
			if err != nil {
				continue
			}

			shouldExclude := false
			for _, pattern := range compiledPatterns {
				if pattern.Match(relativePath) {
					shouldExclude = true
					break
				}
			}
			if shouldExclude {
				continue
			}

			if strings.Contains(strings.ToLower(entry.Name()), searchPattern) {
				mu.Lock()
				results = append(results, fullPath)
				mu.Unlock()
			}

			if entry.IsDir() {
				wg.Add(1)
				go func(path string) {
					semaphore <- struct{}{} // Acquire semaphore
					_ = search(path)
					<-semaphore // Release semaphore
				}(fullPath)
			}
		}

		return nil
	}

	// Start initial search
	wg.Add(1)
	err := search(rootPath)
	if err != nil {
		return nil, err
	}

	// Wait for all goroutines to complete
	wg.Wait()

	return results, nil
}
