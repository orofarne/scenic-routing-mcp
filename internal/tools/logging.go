package tools

import (
	"context"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// logTool wraps a tool handler to log the tool name and execution duration at INFO level.
func logTool[P any](
	name string,
	fn func(context.Context, *mcp.CallToolRequest, P) (*mcp.CallToolResult, any, error),
) func(context.Context, *mcp.CallToolRequest, P) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, p P) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		result, extra, err := fn(ctx, req, p)
		attrs := []any{
			"tool", name,
			"duration_ms", time.Since(start).Milliseconds(),
		}
		if err != nil {
			attrs = append(attrs, "error", err)
		}
		slog.InfoContext(ctx, "tool call", attrs...)
		return result, extra, err
	}
}
