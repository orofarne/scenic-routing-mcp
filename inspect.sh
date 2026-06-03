#!/usr/bin/env bash
# Launches MCP Inspector against the local MCP server.
# Requires the stack to be running: docker compose up -d mcp
set -e
MCP_URL="${MCP_URL:-http://localhost:8080/}"
exec npx -y @modelcontextprotocol/inspector --transport http --server-url "$MCP_URL"
