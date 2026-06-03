// Package handler provides HTTP handlers for the scenic routing server.
package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/orofarne/scenic-routing-mcp/internal/preview"
	"github.com/orofarne/scenic-routing-mcp/internal/routestore"
)

// Preview returns an http.Handler that serves an interactive route preview page.
// Register it at "GET /preview/{id}".
func Preview(store *routestore.Store, tilesURL, tilesAttr string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		params, result, err := store.Load(r.Context(), id)
		if errors.Is(err, routestore.ErrNotFound) {
			http.Error(w, "route not found", http.StatusNotFound)
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "preview: load route", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		html, err := preview.Page(params, result, tilesURL, tilesAttr)
		if err != nil {
			slog.ErrorContext(r.Context(), "preview: generate page", "err", err)
			http.Error(w, "render failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(html))
	})
}
