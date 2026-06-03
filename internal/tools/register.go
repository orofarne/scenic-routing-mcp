package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orofarne/scenic-routing-mcp/internal/geodata"
	"github.com/orofarne/scenic-routing-mcp/internal/ollama"
	"github.com/orofarne/scenic-routing-mcp/internal/routestore"
	"github.com/orofarne/scenic-routing-mcp/internal/valhalla"
)

// Register adds all scenic routing tools to the MCP server.
func Register(s *mcp.Server, geo *geodata.Client, val *valhalla.Client, emb *ollama.Client, store *routestore.Store, publicURL string) {
	registerListTags(s)
	registerScenicRoute(s, geo, val, emb, store, publicURL)
}
