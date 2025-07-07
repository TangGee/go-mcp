package everything

import (
	"context"
	"encoding/base64"
	"fmt"
	"iter"
	"strconv"
	"strings"
	"time"

	"github.com/TangGee/go-mcp"
)

const pageSize = 10

var resourceCompletions = map[string][]string{
	"resourceId": {"1", "2", "3", "4", "5"},
}

func genResources() ([]mcp.Resource, map[string]mcp.ResourceContents) {
	var resources []mcp.Resource
	contents := make(map[string]mcp.ResourceContents)

	for i := 0; i < 100; i++ {
		uri := fmt.Sprintf("test://static/resource/%d", i+1)
		name := fmt.Sprintf("Resource %d", i+1)
		if i%2 == 0 {
			resources = append(resources, mcp.Resource{
				URI:      uri,
				Name:     name,
				MimeType: "text/plain",
			})
			contents[uri] = mcp.ResourceContents{
				URI:      uri,
				MimeType: "text/plain",
				Text:     fmt.Sprintf("Resource %d: This is a plain text resource", i+1),
			}
		} else {
			content := fmt.Sprintf("Resource %d: This is a base64 blob", i+1)
			c64 := base64.StdEncoding.EncodeToString([]byte(content))
			resources = append(resources, mcp.Resource{
				URI:      uri,
				Name:     name,
				MimeType: "application/octet-stream",
			})
			contents[uri] = mcp.ResourceContents{
				URI:      uri,
				MimeType: "application/octet-stream",
				Blob:     c64,
			}
		}
	}

	return resources, contents
}

// ListResources implements mcp.ResourceServer interface.
func (s *Server) ListResources(
	_ context.Context,
	params mcp.ListResourcesParams,
	_ mcp.ProgressReporter,
	_ mcp.RequestClientFunc,
) (mcp.ListResourcesResult, error) {
	s.log(fmt.Sprintf("ListResources: %s", params.Cursor), mcp.LogLevelDebug)

	startIndex := 0
	if params.Cursor != "" {
		startIndex, _ = strconv.Atoi(params.Cursor)
	}
	endIndex := startIndex + pageSize
	rs, _ := genResources()
	if endIndex > len(rs) {
		endIndex = len(rs)
	}
	resources := rs[startIndex:endIndex]

	nextCursor := ""
	if endIndex < len(rs) {
		nextCursor = fmt.Sprintf("%d", endIndex)
	}

	return mcp.ListResourcesResult{
		Resources:  resources,
		NextCursor: nextCursor,
	}, nil
}

// ReadResource implements mcp.ResourceServer interface.
func (s *Server) ReadResource(
	_ context.Context,
	params mcp.ReadResourceParams,
	_ mcp.ProgressReporter,
	_ mcp.RequestClientFunc,
) (mcp.ReadResourceResult, error) {
	s.log(fmt.Sprintf("ReadResource: %s", params.URI), mcp.LogLevelDebug)

	if !strings.HasPrefix(params.URI, "test://static/resource/") {
		return mcp.ReadResourceResult{}, fmt.Errorf("resource not found")
	}

	_, cs := genResources()

	resource, ok := cs[params.URI]
	if !ok {
		return mcp.ReadResourceResult{}, fmt.Errorf("resource not found")
	}

	return mcp.ReadResourceResult{
		Contents: []mcp.ResourceContents{resource},
	}, nil
}

// ListResourceTemplates implements mcp.ResourceServer interface.
func (s *Server) ListResourceTemplates(
	_ context.Context,
	_ mcp.ListResourceTemplatesParams,
	_ mcp.ProgressReporter,
	_ mcp.RequestClientFunc,
) (mcp.ListResourceTemplatesResult, error) {
	s.log("ListResourceTemplates", mcp.LogLevelDebug)

	return mcp.ListResourceTemplatesResult{
		Templates: []mcp.ResourceTemplate{
			{
				URITemplate: "test://static/resource/{id}",
				Name:        "Static Resource",
				Description: "A status resource with numeric ID",
			},
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
			HasMore bool     `json:"hasMore,omitempty"`
			Total   int      `json:"total,omitempty"`
		}{
			Values:  values,
			HasMore: false,
		},
	}, nil
}

// SubscribeResource implements mcp.ResourceSubscriptionHandler interface.
func (s *Server) SubscribeResource(params mcp.SubscribeResourceParams) {
	s.log(fmt.Sprintf("SubscribeResource: %s", params.URI), mcp.LogLevelDebug)

	s.resourceSubscribers.Store(params.URI, struct{}{})
}

// UnsubscribeResource implements mcp.ResourceSubscriptionHandler interface.
func (s *Server) UnsubscribeResource(params mcp.UnsubscribeResourceParams) {
	s.log(fmt.Sprintf("UnsubscribeResource: %s", params.URI), mcp.LogLevelDebug)

	s.resourceSubscribers.Delete(params.URI)
}

// SubscribedResourceUpdates implements mcp.ResourceSubscriptionHandler interface.
func (s *Server) SubscribedResourceUpdates() iter.Seq[string] {
	return func(yield func(string) bool) {
		for {
			select {
			case <-s.done:
				return
			case uri := <-s.updateResourceSubs:
				if !yield(uri) {
					return
				}
			}
		}
	}
}

func (s *Server) simulateResourceUpdates() {
	defer close(s.resourceSubsClosed)

	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
		}

		s.resourceSubscribers.Range(func(key, _ any) bool {
			uri, _ := key.(string)

			s.log(fmt.Sprintf("simulateResourceUpdates: Resource %s updated", uri), mcp.LogLevelDebug)

			select {
			case s.updateResourceSubs <- uri:
			case <-s.done:
				return false
			}

			return true
		})
	}
}
