package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orofarne/scenic-routing-mcp/internal/geodata"
	"github.com/orofarne/scenic-routing-mcp/internal/handler"
	"github.com/orofarne/scenic-routing-mcp/internal/ollama"
	"github.com/orofarne/scenic-routing-mcp/internal/routestore"
	"github.com/orofarne/scenic-routing-mcp/internal/tools"
	"github.com/orofarne/scenic-routing-mcp/internal/valhalla"
)

func main() {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://scenic:scenic@db:5432/scenic"
	}
	valhallaURL := os.Getenv("VALHALLA_URL")
	if valhallaURL == "" {
		valhallaURL = "http://valhalla:8002"
	}
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://ollama:11434"
	}
	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}
	addr := os.Getenv("MCP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	tilesURL := os.Getenv("MAP_TILES_URL")
	tilesAttr := os.Getenv("MAP_TILES_ATTR")
	publicURL := os.Getenv("PUBLIC_URL")
	if publicURL == "" {
		publicURL = "http://localhost:8080"
	}

	routeTTL := time.Hour
	if v := os.Getenv("ROUTE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			routeTTL = d
		} else {
			slog.Warn("invalid ROUTE_TTL, using 1h", "value", v)
		}
	}

	geo, err := geodata.New(ctx, dsn)
	if err != nil {
		slog.Error("geodata init failed", "err", err)
		os.Exit(1)
	}
	defer geo.Close()

	val := valhalla.New(valhallaURL)
	emb := ollama.New(ollamaURL)
	store := routestore.New(redisAddr, routeTTL)

	srv := mcp.NewServer(
		&mcp.Implementation{Name: "scenic-routing", Version: "0.1.0"},
		nil,
	)
	tools.Register(srv, geo, val, emb, store, publicURL)

	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return srv
	}, &mcp.StreamableHTTPOptions{Stateless: true})

	mux := http.NewServeMux()
	mux.Handle("/", mcpHandler)
	mux.Handle("GET /preview/{id}", handler.Preview(store, tilesURL, tilesAttr))
	mux.Handle("GET /export/{file}", handler.Export(store))
	mux.Handle("GET /debug/{id}", handler.Debug(store))

	slog.Info("MCP server listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("http server failed", "err", err)
		os.Exit(1)
	}
}
