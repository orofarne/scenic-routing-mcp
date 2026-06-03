package geodata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Feature is a single OSM object from the feature table.
type Feature struct {
	ID      int64             `json:"id"`
	OsmID   int64             `json:"osm_id"`
	OsmType string            `json:"osm_type"`
	Tags    map[string]string `json:"tags"`
	// Geom is the raw GeoJSON geometry (simplified). May be Point, LineString,
	// Polygon, or Multi* depending on the OSM object type.
	Geom json.RawMessage `json:"geom,omitempty"`
	// Similarity is the combined score [0,1] from the active search signals.
	Similarity float64 `json:"similarity,omitempty"`
}

// Query describes a spatial + semantic + tag search over the feature table.
// At least one of QueryVec, TagInclude, or NameQuery must be set.
type Query struct {
	// Bounding box (required).
	MinLat, MinLon, MaxLat, MaxLon float64

	// QueryVec: 256-dim normalized embedding for semantic (cosine) ranking.
	// nil = semantic ranking disabled.
	QueryVec []float32

	// TagInclude: OSM tag key=value pairs that must all match (AND semantics).
	// Value "*" means the key must exist with any value (tags ? key).
	// Other values use exact containment (tags @> filter::jsonb).
	// nil or empty = no inclusion filter.
	TagInclude map[string]string

	// TagExclude: OSM tag key=value pairs that must NOT match.
	// Value "*" means the key must not exist (NOT tags ? key).
	// Other values exclude that specific value (NOT tags @> filter::jsonb).
	// Applied as a hard WHERE clause after TagInclude.
	// nil or empty = no exclusion filter.
	TagExclude map[string]string

	// NameQuery: fuzzy substring search over name/name:*/description/description:*
	// using pg_trgm word_similarity. "" = disabled.
	NameQuery string

	// MinSim: minimum similarity threshold. 0 = use default per active signals.
	MinSim float64

	// Limit: max results (0 → 1000, max 5000).
	Limit int
}

// Client queries the PostGIS feature table directly.
type Client struct {
	pool *pgxpool.Pool
}

// New opens a connection pool to the given DSN (PostgreSQL connection string).
func New(ctx context.Context, dsn string) (*Client, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("geodata: connect: %w", err)
	}
	return &Client{pool: pool}, nil
}

// Close releases the connection pool.
func (c *Client) Close() {
	c.pool.Close()
}

// Features executes a spatial+semantic+tag query and returns matching features
// ordered by combined similarity score descending.
//
// The query is built dynamically based on which fields of q are set:
//   - QueryVec only:              cosine similarity against poi_vec
//   - NameQuery only:             word_similarity against name/description tags via pg_trgm
//   - TagInclude only:            all matching features, similarity=1.0
//   - QueryVec + NameQuery:       0.6×cosine + 0.4×word_similarity
//   - Any + TagInclude/Exclude:   applied as hard WHERE clauses
//
// The MATERIALIZED CTE ensures the GiST spatial index is applied before the
// cosine/trgm scoring, avoiding full-table scans.
func (c *Client) Features(ctx context.Context, q Query) ([]Feature, error) {
	if q.Limit <= 0 || q.Limit > 5000 {
		q.Limit = 1000
	}

	useVec := len(q.QueryVec) > 0
	useName := q.NameQuery != ""
	useInclude := len(q.TagInclude) > 0
	useExclude := len(q.TagExclude) > 0

	// Default min similarity per active signals.
	minSim := q.MinSim
	if minSim <= 0 {
		switch {
		case useVec && useName:
			minSim = 0.45
		case useVec:
			minSim = 0.55
		case useName:
			minSim = 0.25
		default:
			minSim = 0 // include-only: no threshold
		}
	}

	var args []any
	param := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	bboxExpr := fmt.Sprintf("ST_MakeEnvelope(%s,%s,%s,%s,4326)",
		param(q.MinLon), param(q.MinLat), param(q.MaxLon), param(q.MaxLat))

	// Build expressions for optional signals.
	var vecRef, nameRef string
	if useVec {
		vecRef = param(formatVec(q.QueryVec)) + "::vector"
	}
	if useName {
		nameRef = param(q.NameQuery)
	}

	// Similarity expression — combined score of active signals.
	var simExpr string
	switch {
	case useVec && useName:
		simExpr = fmt.Sprintf("0.6*(1-(poi_vec<=>%s))+0.4*word_similarity(%s,feature_names_text(tags))", vecRef, nameRef)
	case useVec:
		simExpr = fmt.Sprintf("1-(poi_vec<=>%s)", vecRef)
	case useName:
		simExpr = fmt.Sprintf("word_similarity(%s,feature_names_text(tags))", nameRef)
	default:
		simExpr = "1.0::float8"
	}

	// Candidate WHERE clauses (applied before scoring, benefit from indexes).
	var where []string
	where = append(where, "geom IS NOT NULL")
	where = append(where, "geom && "+bboxExpr)
	if useVec {
		where = append(where, "poi_vec IS NOT NULL")
	}
	if useName {
		// <% uses the GIN trgm index; threshold controlled by pg_trgm.word_similarity_threshold.
		where = append(where, fmt.Sprintf("%s <%% feature_names_text(tags)", nameRef))
	}
	if useInclude {
		where = append(where, tagIncludeClauses(q.TagInclude, param)...)
	}
	if useExclude {
		where = append(where, tagExcludeClauses(q.TagExclude, param)...)
	}

	simMinRef := param(minSim)
	limitRef := param(q.Limit)

	sql := fmt.Sprintf(`
		WITH candidates AS MATERIALIZED (
			SELECT id, osm_id, osm_type, tags,
			       ST_AsGeoJSON(ST_SimplifyPreserveTopology(geom, 0.0001)) AS geom_json,
			       poi_vec
			FROM feature
			WHERE %s
		),
		scored AS (
			SELECT id, osm_id, osm_type, tags, geom_json,
			       (%s) AS similarity
			FROM candidates
		)
		SELECT id, osm_id, osm_type, tags, geom_json, similarity
		FROM scored
		WHERE similarity >= %s
		ORDER BY similarity DESC
		LIMIT %s
	`, strings.Join(where, " AND "), simExpr, simMinRef, limitRef)

	rows, err := c.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("geodata: features: %w", err)
	}
	defer rows.Close()

	var features []Feature
	for rows.Next() {
		var f Feature
		var tagsRaw, geomRaw []byte
		if err := rows.Scan(&f.ID, &f.OsmID, &f.OsmType, &tagsRaw, &geomRaw, &f.Similarity); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tagsRaw, &f.Tags); err != nil {
			return nil, fmt.Errorf("geodata: decode tags: %w", err)
		}
		f.Geom = geomRaw
		features = append(features, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// When semantic search is active, drop features whose score falls below
	// vecRelThreshold × top_score. This cuts off irrelevant results that
	// squeeze past the absolute minSim threshold due to the compressed
	// embedding space.
	if useVec && len(features) > 1 {
		const vecRelThreshold = 0.88
		floor := features[0].Similarity * vecRelThreshold
		n := len(features)
		for i := 1; i < n; i++ {
			if features[i].Similarity < floor {
				features = features[:i]
				break
			}
		}
	}

	return features, nil
}

// tagIncludeClauses builds SQL WHERE clauses for TagInclude.
// Value "*" → tags ? key (key exists). Other values → tags @> {k:v}::jsonb.
// All clauses must match (AND semantics).
func tagIncludeClauses(include map[string]string, param func(any) string) []string {
	var clauses []string
	exact := make(map[string]string)
	for k, v := range include {
		if v == "*" {
			clauses = append(clauses, fmt.Sprintf("tags ? %s", param(k)))
		} else {
			exact[k] = v
		}
	}
	if len(exact) > 0 {
		b, _ := json.Marshal(exact)
		clauses = append(clauses, fmt.Sprintf("tags @> %s::jsonb", param(string(b))))
	}
	return clauses
}

// tagExcludeClauses builds SQL WHERE clauses for TagExclude.
// Value "*" → NOT (tags ? key). Other values → NOT (tags @> {k:v}::jsonb).
func tagExcludeClauses(exclude map[string]string, param func(any) string) []string {
	var clauses []string
	exact := make(map[string]string)
	for k, v := range exclude {
		if v == "*" {
			clauses = append(clauses, fmt.Sprintf("NOT (tags ? %s)", param(k)))
		} else {
			exact[k] = v
		}
	}
	if len(exact) > 0 {
		b, _ := json.Marshal(exact)
		clauses = append(clauses, fmt.Sprintf("NOT (tags @> %s::jsonb)", param(string(b))))
	}
	return clauses
}

// formatVec formats a float32 slice as a PostgreSQL vector literal "[f1,f2,...]".
func formatVec(v []float32) string {
	b := make([]byte, 0, len(v)*8+2)
	b = append(b, '[')
	for i, f := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = fmt.Appendf(b, "%g", f)
	}
	b = append(b, ']')
	return string(b)
}
