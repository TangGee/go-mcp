package everything

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MegaGrindStone/go-mcp/pkg/mcp"
)

const pageSize = 10

var resourceCompletions = map[string][]string{
	"resourceId": {"1", "2", "3", "4", "5"},
}

func genResources() []mcp.Resource {
	var resources []mcp.Resource
	for i := 0; i < 100; i++ {
		uri := fmt.Sprintf("test://static/resource/%d", i+1)
		name := fmt.Sprintf("Resource %d", i+1)
		if i%2 == 0 {
			resources = append(resources, mcp.Resource{
				URI:      uri,
				Name:     name,
				MimeType: "text/plain",
				Text:     fmt.Sprintf("Resource %d: This is a plain text resource", i+1),
			})
		} else {
			content := fmt.Sprintf("Resource %d: This is a base64 blob", i+1)
			c64 := base64.StdEncoding.EncodeToString([]byte(content))
			resources = append(resources, mcp.Resource{
				URI:      uri,
				Name:     name,
				MimeType: "application/octet-stream",
				Blob:     c64,
			})
		}
	}

	return resources
}

// ListResources implements mcp.ResourceServer interface.
func (s *Server) ListResources(
	_ context.Context,
	params mcp.ListResourcesParams,
	_ mcp.RequestClientFunc,
) (mcp.ResourceList, error) {
	s.log(fmt.Sprintf("ListResources: %s", params.Cursor), mcp.LogLevelDebug)

	startIndex := 0
	if params.Cursor != "" {
		startIndex, _ = strconv.Atoi(params.Cursor)
	}
	endIndex := startIndex + pageSize
	if endIndex > len(genResources()) {
		endIndex = len(genResources())
	}
	resources := genResources()[startIndex:endIndex]

	nextCursor := ""
	if endIndex < len(genResources()) {
		nextCursor = fmt.Sprintf("%d", endIndex)
	}

	return mcp.ResourceList{
		Resources:  resources,
		NextCursor: nextCursor,
	}, nil
}

// ReadResource implements mcp.ResourceServer interface.
func (s *Server) ReadResource(
	_ context.Context,
	params mcp.ReadResourceParams,
	_ mcp.RequestClientFunc,
) (mcp.Resource, error) {
	s.log(fmt.Sprintf("ReadResource: %s", params.URI), mcp.LogLevelDebug)

	if !strings.HasPrefix(params.URI, "test://static/resource/") {
		return mcp.Resource{}, fmt.Errorf("resource not found")
	}
	arrStr := strings.Split(params.URI, "/")
	if len(arrStr) < 2 {
		return mcp.Resource{}, fmt.Errorf("resource not found")
	}

	index, _ := strconv.Atoi(arrStr[len(arrStr)-1])
	if index < 0 || index >= len(genResources()) {
		return mcp.Resource{}, fmt.Errorf("resource not found")
	}

	resource := genResources()[index]
	return resource, nil
}

// ListResourceTemplates implements mcp.ResourceServer interface.
func (s *Server) ListResourceTemplates(
	_ context.Context,
	_ mcp.ListResourceTemplatesParams,
	_ mcp.RequestClientFunc,
) ([]mcp.ResourceTemplate, error) {
	s.log("ListResourceTemplates", mcp.LogLevelDebug)

	return []mcp.ResourceTemplate{
		{
			URITemplate: "test://static/resource/{id}",
			Name:        "Static Resource",
			Description: "A status resource with numeric ID",
		},
	}, nil
}

// CompletesResourceTemplate implements mcp.ResourceServer interface.
func (s *Server) CompletesResourceTemplate(
	_ context.Context,
	params mcp.CompletesCompletionParams,
	_ mcp.RequestClientFunc,
) (mcp.CompletionResult, error) {
	s.log(fmt.Sprintf("CompletesResourceTemplate: %s", params.Ref.Name), mcp.LogLevelDebug)

	completions, ok := resourceCompletions[params.Ref.Name]
	if !ok {
		return mcp.CompletionResult{}, nil
	}

	var values []string
	for _, c := range completions {
		if strings.HasPrefix(c, params.Argument.Value) {
			values = append(values, c)
		}
	}

	return mcp.CompletionResult{
		Completion: struct {
			Values  []string `json:"values"`
			HasMore bool     `json:"hasMore"`
		}{
			Values:  values,
			HasMore: false,
		},
	}, nil
}

// SubscribeResource implements mcp.ResourceServer interface.
func (s *Server) SubscribeResource(params mcp.ResourcesSubscribeParams) {
	s.log(fmt.Sprintf("SubscribeResource: %s", params.URI), mcp.LogLevelDebug)

	s.resourceSubscribers.Store(params.URI, struct{}{})
}

// UnsubscribeResource implements mcp.ResourceServer interface.
func (s *Server) UnsubscribeResource(params mcp.ResourcesSubscribeParams) {
	s.log(fmt.Sprintf("UnsubscribeResource: %s", params.URI), mcp.LogLevelDebug)

	s.resourceSubscribers.Delete(params.URI)
}

// ResourceSubscribedUpdates implements mcp.ResourceSubscribedUpdater interface.
func (s *Server) ResourceSubscribedUpdates() <-chan string {
	return s.updateResourceSubsChan
}

func (s *Server) simulateResourceUpdates() {
	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-s.doneChan:
			return
		case <-ticker.C:
		}

		s.resourceSubscribers.Range(func(key, _ any) bool {
			uri, _ := key.(string)

			s.log(fmt.Sprintf("simulateResourceUpdates: Resource %s updated", uri), mcp.LogLevelDebug)

			select {
			case s.updateResourceSubsChan <- uri:
			case <-s.doneChan:
				return false
			}

			return true
		})
	}
}
