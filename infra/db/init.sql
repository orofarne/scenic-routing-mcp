CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS hstore;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Single table for all scenic features imported from OSM.
-- The planning layer decides how to use each feature based on its tags and geometry.
--
-- tags     — all OSM tags of the object; use GIN index for any key/value query
-- geom     — original geometry: Point / LineString / Polygon / MultiPolygon
-- osm_type — OSM primitive: 'node', 'way', or 'relation'
-- poi_vec  — 256-dim nomic-embed-text-v1 embedding of the feature's text profile;
--            NULL until embed_features.py runs after each import
CREATE TABLE IF NOT EXISTS feature (
  id          BIGSERIAL PRIMARY KEY,
  osm_id      BIGINT NOT NULL,
  osm_type    TEXT NOT NULL,
  tags        JSONB NOT NULL,
  geom        GEOMETRY(Geometry, 4326),
  updated_at  TIMESTAMP DEFAULT now(),
  poi_vec     vector(256)
);

CREATE INDEX IF NOT EXISTS feature_tags_idx ON feature USING GIN(tags);
CREATE INDEX IF NOT EXISTS feature_geom_idx ON feature USING GIST(geom);
-- HNSW index for approximate nearest-neighbour search on poi_vec (cosine similarity).
-- Built once the table is populated; automatically maintained on INSERT/UPDATE.
CREATE INDEX IF NOT EXISTS feature_vec_idx ON feature USING hnsw(poi_vec vector_cosine_ops);

-- Concatenate all name/description tags (including name:*, description:*) for trigram search.
-- Language-agnostic; covers multilingual OSM data out of the box.
CREATE OR REPLACE FUNCTION feature_names_text(tags jsonb) RETURNS text
LANGUAGE sql IMMUTABLE STRICT AS $$
    SELECT lower(string_agg(v, ' '))
    FROM jsonb_each_text(tags) AS t(k, v)
    WHERE k = 'name' OR k LIKE 'name:%'
       OR k = 'description' OR k LIKE 'description:%'
$$;

CREATE INDEX IF NOT EXISTS feature_names_trgm_idx
    ON feature USING GIN(feature_names_text(tags) gin_trgm_ops);
