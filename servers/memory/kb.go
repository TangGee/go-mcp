package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type knowledgeBase struct {
	memoryFilePath string
}

type kbItem struct {
	Type string `json:"type"`

	// For Type == "entity"
	Name         string   `json:"name,omitempty"`
	EntityType   string   `json:"entityType,omitempty"`
	Observations []string `json:"observations,omitempty"`

	// For Type == "relation"
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	RelationType string `json:"relationType,omitempty"`
}

type knowledgeGraph struct {
	Entities  []entity   `json:"entities"`
	Relations []relation `json:"relations"`
}

func newKnowledgeBase(memoryFilePath string) knowledgeBase {
	return knowledgeBase{
		memoryFilePath: memoryFilePath,
	}
}

func (k knowledgeBase) loadGraph() (knowledgeGraph, error) {
	data, err := os.ReadFile(k.memoryFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return knowledgeGraph{}, nil
		}
		return knowledgeGraph{}, fmt.Errorf("failed to read file %s: %w", k.memoryFilePath, err)
	}

	var items []kbItem
	if err := json.Unmarshal(data, &items); err != nil {
		return knowledgeGraph{}, fmt.Errorf("failed to unmarshal file %s: %w", k.memoryFilePath, err)
	}

	graph := knowledgeGraph{
		Entities:  []entity{},
		Relations: []relation{},
	}

	for _, item := range items {
		switch item.Type {
		case "entity":
			graph.Entities = append(graph.Entities, entity{
				Name:         item.Name,
				EntityType:   item.EntityType,
				Observations: item.Observations,
			})
		case "relation":
			graph.Relations = append(graph.Relations, relation{
				From:         item.From,
				To:           item.To,
				RelationType: item.RelationType,
			})
		}
	}

	return graph, nil
}

func (k knowledgeBase) saveGraph(graph knowledgeGraph) error {
	items := make([]kbItem, 0, len(graph.Entities)+len(graph.Relations))

	for _, entity := range graph.Entities {
		items = append(items, kbItem{
			Type:         "entity",
			Name:         entity.Name,
			EntityType:   entity.EntityType,
			Observations: entity.Observations,
		})
	}

	for _, relation := range graph.Relations {
		items = append(items, kbItem{
			Type:         "relation",
			From:         relation.From,
			To:           relation.To,
			RelationType: relation.RelationType,
		})
	}

	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}

	return os.WriteFile(k.memoryFilePath, itemsJSON, 0600)
}

func (k knowledgeBase) createEntities(entities []entity) ([]entity, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return nil, err
	}

	var newEntities []entity
	for _, entity := range entities {
		exists := false
		for _, existingEntity := range graph.Entities {
			if existingEntity.Name == entity.Name {
				exists = true
				break
			}
		}

		if !exists {
			newEntities = append(newEntities, entity)
			graph.Entities = append(graph.Entities, entity)
		}
	}

	if err := k.saveGraph(graph); err != nil {
		return nil, err
	}

	return newEntities, nil
}

func (k knowledgeBase) createRelations(relations []relation) ([]relation, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return nil, err
	}

	var newRelations []relation
	for _, relation := range relations {
		exists := false
		for _, existingRelation := range graph.Relations {
			if existingRelation.From == relation.From &&
				existingRelation.To == relation.To &&
				existingRelation.RelationType == relation.RelationType {
				exists = true
				break
			}
		}

		if !exists {
			newRelations = append(newRelations, relation)
			graph.Relations = append(graph.Relations, relation)
		}
	}

	if err := k.saveGraph(graph); err != nil {
		return nil, err
	}

	return newRelations, nil
}

func (k knowledgeBase) addObservations(observations []observation) ([]observation, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return nil, err
	}

	var results []observation

	for _, obs := range observations {
		entityIndex := -1
		for i, entity := range graph.Entities {
			if entity.Name == obs.EntityName {
				entityIndex = i
				break
			}
		}

		if entityIndex == -1 {
			return nil, fmt.Errorf("entity with name %s not found", obs.EntityName)
		}

		var newObservations []string
		for _, content := range obs.Contents {
			exists := false
			for _, existingObservation := range graph.Entities[entityIndex].Observations {
				if existingObservation == content {
					exists = true
					break
				}
			}

			if !exists {
				newObservations = append(newObservations, content)
				graph.Entities[entityIndex].Observations = append(graph.Entities[entityIndex].Observations, content)
			}
		}

		results = append(results, observation{
			EntityName: obs.EntityName,
			Contents:   newObservations,
		})
	}

	if err := k.saveGraph(graph); err != nil {
		return nil, err
	}

	return results, nil
}

func (k knowledgeBase) deleteEntities(entityNames []string) error {
	graph, err := k.loadGraph()
	if err != nil {
		return err
	}

	// Create map for quick lookup
	entitiesToDelete := make(map[string]bool)
	for _, name := range entityNames {
		entitiesToDelete[name] = true
	}

	// Filter entities
	var filteredEntities []entity
	for _, entity := range graph.Entities {
		if !entitiesToDelete[entity.Name] {
			filteredEntities = append(filteredEntities, entity)
		}
	}
	graph.Entities = filteredEntities

	// Filter relations
	var filteredRelations []relation
	for _, relation := range graph.Relations {
		if !entitiesToDelete[relation.From] && !entitiesToDelete[relation.To] {
			filteredRelations = append(filteredRelations, relation)
		}
	}
	graph.Relations = filteredRelations

	return k.saveGraph(graph)
}

func (k knowledgeBase) deleteObservations(deletions []observation) error {
	graph, err := k.loadGraph()
	if err != nil {
		return err
	}

	for _, deletion := range deletions {
		for i, entity := range graph.Entities {
			if entity.Name == deletion.EntityName {
				// Create a map for quick lookup
				observationsToDelete := make(map[string]bool)
				for _, observation := range deletion.Observations {
					observationsToDelete[observation] = true
				}

				// Filter observations
				var filteredObservations []string
				for _, observation := range entity.Observations {
					if !observationsToDelete[observation] {
						filteredObservations = append(filteredObservations, observation)
					}
				}

				graph.Entities[i].Observations = filteredObservations
				break
			}
		}
	}

	return k.saveGraph(graph)
}

func (k knowledgeBase) deleteRelations(relations []relation) error {
	graph, err := k.loadGraph()
	if err != nil {
		return err
	}

	var filteredRelations []relation
	for _, existingRelation := range graph.Relations {
		shouldKeep := true

		for _, relationToDelete := range relations {
			if existingRelation.From == relationToDelete.From &&
				existingRelation.To == relationToDelete.To &&
				existingRelation.RelationType == relationToDelete.RelationType {
				shouldKeep = false
				break
			}
		}

		if shouldKeep {
			filteredRelations = append(filteredRelations, existingRelation)
		}
	}

	graph.Relations = filteredRelations
	return k.saveGraph(graph)
}

func (k knowledgeBase) readGraph() (knowledgeGraph, error) {
	return k.loadGraph()
}

func (k knowledgeBase) searchNodes(query string) (knowledgeGraph, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return knowledgeGraph{}, err
	}

	queryLower := strings.ToLower(query)
	var filteredEntities []entity

	// Filter entities
	for _, entity := range graph.Entities {
		if strings.Contains(strings.ToLower(entity.Name), queryLower) ||
			strings.Contains(strings.ToLower(entity.EntityType), queryLower) {
			filteredEntities = append(filteredEntities, entity)
			continue
		}

		// Check observations
		for _, observation := range entity.Observations {
			if strings.Contains(strings.ToLower(observation), queryLower) {
				filteredEntities = append(filteredEntities, entity)
				break
			}
		}
	}

	// Create map for quick entity lookup
	filteredEntityNames := make(map[string]bool)
	for _, entity := range filteredEntities {
		filteredEntityNames[entity.Name] = true
	}

	// Filter relations
	var filteredRelations []relation
	for _, relation := range graph.Relations {
		if filteredEntityNames[relation.From] && filteredEntityNames[relation.To] {
			filteredRelations = append(filteredRelations, relation)
		}
	}

	return knowledgeGraph{
		Entities:  filteredEntities,
		Relations: filteredRelations,
	}, nil
}

func (k knowledgeBase) openNodes(names []string) (knowledgeGraph, error) {
	graph, err := k.loadGraph()
	if err != nil {
		return knowledgeGraph{}, err
	}

	// Create map for quick name lookup
	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}

	// Filter entities
	var filteredEntities []entity
	for _, entity := range graph.Entities {
		if nameSet[entity.Name] {
			filteredEntities = append(filteredEntities, entity)
		}
	}

	// Create map for quick entity lookup
	filteredEntityNames := make(map[string]bool)
	for _, entity := range filteredEntities {
		filteredEntityNames[entity.Name] = true
	}

	// Filter relations
	var filteredRelations []relation
	for _, relation := range graph.Relations {
		if filteredEntityNames[relation.From] && filteredEntityNames[relation.To] {
			filteredRelations = append(filteredRelations, relation)
		}
	}

	return knowledgeGraph{
		Entities:  filteredEntities,
		Relations: filteredRelations,
	}, nil
}
