package mcpserver

import (
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
	"strings"
)

type guidePageInput struct {
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque continuation for the exact current publication"`
}
type guideSearchInput struct {
	Query string `json:"query" jsonschema:"Search text, 2 to 160 characters"`
}
type guideArticleInput struct {
	Slug   string `json:"slug" jsonschema:"Exact published Guide slug from navigation/search"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Publication-and-article-bound continuation"`
}

func addGuideTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[guidePageInput]{name: "authrail_get_guide_navigation", description: "Browse bounded navigation from the published tenant Guide; internal staff guidance is excluded.", scope: Scope, method: http.MethodGet, readonly: true, prepare: func(input guidePageInput) (string, any, error) {
		if len(input.Cursor) > 2000 {
			return "", nil, errors.New("Use the returned Guide cursor")
		}
		q := url.Values{}
		if input.Cursor != "" {
			q.Set("cursor", input.Cursor)
		}
		return "/organisation/mcp/guide/navigation?" + q.Encode(), nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[guideSearchInput]{name: "authrail_search_guide", description: "Search the current published tenant Guide with bounded results.", scope: Scope, method: http.MethodGet, readonly: true, prepare: func(input guideSearchInput) (string, any, error) {
		if len(strings.TrimSpace(input.Query)) < 2 || len(input.Query) > 640 {
			return "", nil, errors.New("Use 2 to 160 search characters")
		}
		return "/organisation/mcp/guide/search?" + url.Values{"query": {input.Query}}.Encode(), nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[guideArticleInput]{name: "authrail_get_guide_article", description: "Read a published tenant Guide article in bounded section chunks. Continue only with its exact publication-bound cursor; changed publications require a fresh read.", scope: Scope, method: http.MethodGet, readonly: true, prepare: func(input guideArticleInput) (string, any, error) {
		if input.Slug == "" || len(input.Slug) > 240 || len(input.Cursor) > 2000 || strings.Contains(input.Slug, "..") || strings.ContainsAny(input.Slug, "?%#\\") {
			return "", nil, errors.New("Choose an exact published article slug")
		}
		q := url.Values{}
		if input.Cursor != "" {
			q.Set("cursor", input.Cursor)
		}
		return "/organisation/mcp/guide/articles/" + input.Slug + "?" + q.Encode(), nil, nil
	}})
}
