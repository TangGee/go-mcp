package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNewKnowledgeBase(t *testing.T) {
	kb := newKnowledgeBase("test.json")
	if kb.memoryFilePath != "test.json" {
		t.Errorf("expected memoryFilePath to be 'test.json', got %s", kb.memoryFilePath)
	}
}

//nolint:gocyclo // This is a test covering multiple operations
func TestKnowledgeBaseOperations(t *testing.T) {
	// Create temporary file for testing
	tempDir, err := os.MkdirTemp("", "kb-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	memoryFile := filepath.Join(tempDir, "test-memory.json")
	kb := newKnowledgeBase(memoryFile)

	// Test empty graph
	graph, err := kb.loadGraph()
	if err != nil {
		t.Fatalf("failed to load empty graph: %v", err)
	}
	if len(graph.Entities) != 0 || len(graph.Relations) != 0 {
		t.Errorf("expected empty graph, got %+v", graph)
	}

	// Test creating entities
	testEntities := []entity{
		{
			Name:         "Alice",
			EntityType:   "Person",
			Observations: []string{"Likes coffee"},
		},
		{
			Name:         "Bob",
			EntityType:   "Person",
			Observations: []string{"Likes tea"},
		},
	}

	createdEntities, err := kb.createEntities(testEntities)
	if err != nil {
		t.Fatalf("failed to create entities: %v", err)
	}
	if len(createdEntities) != 2 {
		t.Errorf("expected 2 created entities, got %d", len(createdEntities))
	}

	// Test reading graph
	graph, err = kb.readGraph()
	if err != nil {
		t.Fatalf("failed to read graph: %v", err)
	}
	if len(graph.Entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(graph.Entities))
	}

	// Test creating relations
	testRelations := []relation{
		{
			From:         "Alice",
			To:           "Bob",
			RelationType: "friend",
		},
	}

	createdRelations, err := kb.createRelations(testRelations)
	if err != nil {
		t.Fatalf("failed to create relations: %v", err)
	}
	if len(createdRelations) != 1 {
		t.Errorf("expected 1 created relation, got %d", len(createdRelations))
	}

	// Test adding observations
	testObservations := []observation{
		{
			EntityName: "Alice",
			Contents:   []string{"Works as developer", "Lives in New York"},
		},
	}

	addedObservations, err := kb.addObservations(testObservations)
	if err != nil {
		t.Fatalf("failed to add observations: %v", err)
	}
	if len(addedObservations) != 1 || len(addedObservations[0].Contents) != 2 {
		t.Errorf("expected 1 observation with 2 contents, got %+v", addedObservations)
	}

	// Test searching nodes
	searchResult, err := kb.searchNodes("developer")
	if err != nil {
		t.Fatalf("failed to search nodes: %v", err)
	}
	if len(searchResult.Entities) != 1 || searchResult.Entities[0].Name != "Alice" {
		t.Errorf("expected to find Alice when searching for 'developer', got %+v", searchResult)
	}

	// Test opening specific nodes
	openResult, err := kb.openNodes([]string{"Bob"})
	if err != nil {
		t.Fatalf("failed to open nodes: %v", err)
	}
	if len(openResult.Entities) != 1 || openResult.Entities[0].Name != "Bob" {
		t.Errorf("expected to find Bob when opening 'Bob', got %+v", openResult)
	}

	// Test deleting observations
	deleteObs := []observation{
		{
			EntityName:   "Alice",
			Observations: []string{"Works as developer"},
		},
	}
	err = kb.deleteObservations(deleteObs)
	if err != nil {
		t.Fatalf("failed to delete observations: %v", err)
	}

	// Verify observation was deleted
	graph, _ = kb.readGraph()
	aliceFound := false
	for _, e := range graph.Entities {
		if e.Name == "Alice" {
			aliceFound = true
			for _, obs := range e.Observations {
				if obs == "Works as developer" {
					t.Errorf("observation 'Works as developer' should have been deleted")
				}
			}
		}
	}
	if !aliceFound {
		t.Errorf("entity 'Alice' not found after deleting observation")
	}

	// Test deleting relations
	err = kb.deleteRelations(testRelations)
	if err != nil {
		t.Fatalf("failed to delete relations: %v", err)
	}

	// Verify relation was deleted
	graph, _ = kb.readGraph()
	if len(graph.Relations) != 0 {
		t.Errorf("expected 0 relations after deletion, got %d", len(graph.Relations))
	}

	// Test deleting entities
	err = kb.deleteEntities([]string{"Alice"})
	if err != nil {
		t.Fatalf("failed to delete entities: %v", err)
	}

	// Verify entity was deleted
	graph, _ = kb.readGraph()
	if len(graph.Entities) != 1 || graph.Entities[0].Name != "Bob" {
		t.Errorf("expected only Bob to remain after deleting Alice, got %+v", graph.Entities)
	}
}

func TestSaveAndLoadGraph(t *testing.T) {
	// Create temporary file for testing
	tempDir, err := os.MkdirTemp("", "kb-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	memoryFile := filepath.Join(tempDir, "test-memory.json")
	kb := newKnowledgeBase(memoryFile)

	// Create test graph
	testGraph := knowledgeGraph{
		Entities: []entity{
			{
				Name:         "Charlie",
				EntityType:   "Person",
				Observations: []string{"Likes hiking"},
			},
		},
		Relations: []relation{
			{
				From:         "Charlie",
				To:           "Mountains",
				RelationType: "enjoys",
			},
		},
	}

	// Save graph
	err = kb.saveGraph(testGraph)
	if err != nil {
		t.Fatalf("failed to save graph: %v", err)
	}

	// Load graph
	loadedGraph, err := kb.loadGraph()
	if err != nil {
		t.Fatalf("failed to load graph: %v", err)
	}

	// Check if loaded graph matches test graph
	if !reflect.DeepEqual(testGraph, loadedGraph) {
		t.Errorf("loaded graph does not match saved graph.\nExpected: %+v\nGot: %+v", testGraph, loadedGraph)
	}

	// Test invalid JSON
	err = os.WriteFile(memoryFile, []byte("invalid json"), 0600)
	if err != nil {
		t.Fatalf("failed to write invalid json: %v", err)
	}

	_, err = kb.loadGraph()
	if err == nil {
		t.Errorf("expected error when loading invalid JSON, got nil")
	}
}

func TestDuplicateEntitiesAndRelations(t *testing.T) {
	// Create temporary file for testing
	tempDir, err := os.MkdirTemp("", "kb-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	memoryFile := filepath.Join(tempDir, "test-memory.json")
	kb := newKnowledgeBase(memoryFile)

	// Create initial entities
	initialEntities := []entity{
		{
			Name:         "Dave",
			EntityType:   "Person",
			Observations: []string{"Plays guitar"},
		},
	}

	_, err = kb.createEntities(initialEntities)
	if err != nil {
		t.Fatalf("failed to create initial entities: %v", err)
	}

	// Try to create duplicate entities
	duplicateEntities := []entity{
		{
			Name:         "Dave",
			EntityType:   "Person",
			Observations: []string{"Sings well"},
		},
		{
			Name:         "Eve",
			EntityType:   "Person",
			Observations: []string{"Plays piano"},
		},
	}

	newEntities, err := kb.createEntities(duplicateEntities)
	if err != nil {
		t.Fatalf("failed when adding duplicate entities: %v", err)
	}

	// Should only create Eve, not Dave (duplicate)
	if len(newEntities) != 1 || newEntities[0].Name != "Eve" {
		t.Errorf("expected only 'Eve' to be created, got %+v", newEntities)
	}

	// Create initial relation
	initialRelation := []relation{
		{
			From:         "Dave",
			To:           "Eve",
			RelationType: "friend",
		},
	}

	_, err = kb.createRelations(initialRelation)
	if err != nil {
		t.Fatalf("failed to create initial relation: %v", err)
	}

	// Try to create duplicate relation
	duplicateRelations := []relation{
		{
			From:         "Dave",
			To:           "Eve",
			RelationType: "friend",
		},
		{
			From:         "Eve",
			To:           "Dave",
			RelationType: "friend",
		},
	}

	newRelations, err := kb.createRelations(duplicateRelations)
	if err != nil {
		t.Fatalf("failed when adding duplicate relations: %v", err)
	}

	// Should only create the Eve->Dave relation, not Dave->Eve (duplicate)
	if len(newRelations) != 1 || newRelations[0].From != "Eve" || newRelations[0].To != "Dave" {
		t.Errorf("expected only 'Eve->Dave' relation to be created, got %+v", newRelations)
	}
}

func TestErrorHandling(t *testing.T) {
	// Test with non-existent directory
	kb := newKnowledgeBase("/nonexistent/directory/file.json")

	testEntities := []entity{
		{
			Name:         "TestEntity",
			EntityType:   "Test",
			Observations: []string{"Test observation"},
		},
	}

	// This should fail because the directory doesn't exist
	_, err := kb.createEntities(testEntities)
	if err == nil {
		t.Errorf("expected error when writing to non-existent directory, got nil")
	}

	// Test adding observations to non-existent entity
	kb = newKnowledgeBase(os.TempDir() + "/test-kb.json")

	// Create the file first
	_, err = kb.createEntities(testEntities)
	if err != nil {
		t.Fatalf("failed to create test entity: %v", err)
	}

	// Try to add observation to non-existent entity
	nonExistentObs := []observation{
		{
			EntityName: "NonExistentEntity",
			Contents:   []string{"This shouldn't work"},
		},
	}

	_, err = kb.addObservations(nonExistentObs)
	if err == nil {
		t.Errorf("expected error when adding observations to non-existent entity, got nil")
	}

	// Clean up
	os.Remove(os.TempDir() + "/test-kb.json")
}

func TestFileFormatting(t *testing.T) {
	// Create temporary file for testing
	tempDir, err := os.MkdirTemp("", "kb-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	memoryFile := filepath.Join(tempDir, "test-memory.json")
	kb := newKnowledgeBase(memoryFile)

	// Create entities
	testEntities := []entity{
		{
			Name:         "FileTest",
			EntityType:   "TestEntity",
			Observations: []string{"Test observation"},
		},
	}

	_, err = kb.createEntities(testEntities)
	if err != nil {
		t.Fatalf("failed to create test entity: %v", err)
	}

	// Read file contents directly
	fileData, err := os.ReadFile(memoryFile)
	if err != nil {
		t.Fatalf("failed to read memory file: %v", err)
	}

	// Parse JSON to verify structure
	var items []kbItem
	err = json.Unmarshal(fileData, &items)
	if err != nil {
		t.Fatalf("failed to parse memory file JSON: %v", err)
	}

	// Verify format
	if len(items) != 1 {
		t.Fatalf("expected 1 item in memory file, got %d", len(items))
	}

	item := items[0]
	if item.Type != "entity" ||
		item.Name != "FileTest" ||
		item.EntityType != "TestEntity" ||
		len(item.Observations) != 1 ||
		item.Observations[0] != "Test observation" {
		t.Errorf("memory file item format incorrect: %+v", item)
	}
}
