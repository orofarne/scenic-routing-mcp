package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/orofarne/scenic-routing-mcp/internal/format"
	"github.com/orofarne/scenic-routing-mcp/internal/routestore"

)

// Debug returns an http.Handler that serves a debug GeoJSON FeatureCollection.
// Register it at "GET /debug/{id}".
func Debug(store *routestore.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		_, result, err := store.Load(r.Context(), id)
		if errors.Is(err, routestore.ErrNotFound) {
			http.Error(w, "route not found", http.StatusNotFound)
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "debug: load route", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		body, err := format.DebugGeoJSON(result.BaseRoute, result.ScenicRoute, result.Features,
			result.MinLon, result.MinLat, result.MaxLon, result.MaxLat,
			result.HeatGrid, result.Peaks, result.UsedPeaks)
		if err != nil {
			slog.ErrorContext(r.Context(), "debug: encode geojson", "err", err)
			http.Error(w, "encode failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/geo+json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(body))
	})
}

// Export returns an http.Handler that serves route files (GPX or GeoJSON).
// Register it at "GET /export/{file}" where file is "{id}.gpx" or "{id}.geojson".
func Export(store *routestore.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		var id, ext string
		if after, ok := strings.CutSuffix(file, ".gpx"); ok {
			id, ext = after, "gpx"
		} else if after, ok := strings.CutSuffix(file, ".geojson"); ok {
			id, ext = after, "geojson"
		} else {
			http.Error(w, "unsupported format (use .gpx or .geojson)", http.StatusBadRequest)
			return
		}

		_, result, err := store.Load(r.Context(), id)
		if errors.Is(err, routestore.ErrNotFound) {
			http.Error(w, "route not found", http.StatusNotFound)
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "export: load route", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		switch ext {
		case "gpx":
			w.Header().Set("Content-Type", "application/gpx+xml; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="route.gpx"`)
			w.Header().Set("Cache-Control", "public, max-age=3600")
			_, _ = w.Write([]byte(format.GPX(result.ScenicRoute)))
		case "geojson":
			body, err := format.GeoJSON(result.ScenicRoute)
			if err != nil {
				slog.ErrorContext(r.Context(), "export: encode geojson", "err", err)
				http.Error(w, "encode failed", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/geo+json; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="route.geojson"`)
			w.Header().Set("Cache-Control", "public, max-age=3600")
			_, _ = w.Write([]byte(body))
		}
	})
}
