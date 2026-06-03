package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orofarne/scenic-routing-mcp/internal/dictionary"
)

type listTagsOutput struct {
	Tags []dictionary.Entry `json:"tags"`
}

func registerListTags(s *mcp.Server) {
	entries, err := dictionary.Load()
	if err != nil {
		panic(fmt.Sprintf("list_tags: load dictionary: %v", err))
	}
	payload, err := json.Marshal(listTagsOutput{Tags: entries})
	if err != nil {
		panic(fmt.Sprintf("list_tags: marshal: %v", err))
	}
	text := string(payload)
	out := listTagsOutput{Tags: entries}

	mcp.AddTool(s, &mcp.Tool{
		Name: "list_tags",
		Description: `Returns all OSM tag key=value pairs available in the scenic feature database,
with English descriptions. Use this to discover valid values for poi_include and poi_exclude
before calling plan_scenic_route.

The response is a JSON object with a "tags" array of {key, value, description} entries.
Example: {"tags": [{"key":"amenity","value":"toilets","description":"..."},  ...]}`,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, *listTagsOutput, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, &out, nil
	})
}
