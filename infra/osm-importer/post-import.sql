-- Merge the three osm2pgsql raw tables into the single `feature` table.
-- Run after every import (full) and after every incremental update.
-- Full rebuild inside a single transaction — atomic, no empty-table window (PostgreSQL MVCC).
--
-- Vector carry-over: poi_vec is preserved for objects whose tags haven't changed
-- (matched by osm_id + osm_type + md5(tags)). Objects with changed or new tags
-- get poi_vec = NULL and are picked up by embed_features.py in the next step.

BEGIN;

-- Snapshot existing vectors before truncating.
-- md5(tags::text) is stable because JSONB normalises key ordering.
CREATE TEMP TABLE old_vecs AS
    SELECT osm_id, osm_type, poi_vec, md5(tags::text) AS tags_hash
    FROM feature
    WHERE poi_vec IS NOT NULL;

TRUNCATE feature;

-- Nodes
INSERT INTO feature (osm_id, osm_type, tags, geom, updated_at, poi_vec)
SELECT n.osm_id, n.osm_type, n.tags::jsonb, n.geom, now(), v.poi_vec
FROM raw_node n
LEFT JOIN old_vecs v ON v.osm_id    = n.osm_id
                     AND v.osm_type  = n.osm_type
                     AND v.tags_hash = md5(n.tags::text);

-- Open ways (rivers, canals, ...)
INSERT INTO feature (osm_id, osm_type, tags, geom, updated_at, poi_vec)
SELECT n.osm_id, n.osm_type, n.tags::jsonb, n.geom, now(), v.poi_vec
FROM raw_line n
LEFT JOIN old_vecs v ON v.osm_id    = n.osm_id
                     AND v.osm_type  = n.osm_type
                     AND v.tags_hash = md5(n.tags::text)
WHERE ST_IsValid(n.geom);

-- Polygons / MultiPolygons (parks, forests, water bodies, ...): drop slivers under 50 m²
INSERT INTO feature (osm_id, osm_type, tags, geom, updated_at, poi_vec)
SELECT n.osm_id, n.osm_type, n.tags::jsonb, n.geom, now(), v.poi_vec
FROM raw_poly n
LEFT JOIN old_vecs v ON v.osm_id    = n.osm_id
                     AND v.osm_type  = n.osm_type
                     AND v.tags_hash = md5(n.tags::text)
WHERE ST_IsValid(n.geom)
  AND ST_Area(n.geom::geography) > 50;

-- Named route relations (Thames Path, Capital Ring, etc.)
INSERT INTO feature (osm_id, osm_type, tags, geom, updated_at, poi_vec)
SELECT n.osm_id, n.osm_type, n.tags::jsonb, n.geom, now(), v.poi_vec
FROM raw_route n
LEFT JOIN old_vecs v ON v.osm_id    = n.osm_id
                     AND v.osm_type  = n.osm_type
                     AND v.tags_hash = md5(n.tags::text)
WHERE ST_IsValid(n.geom);

COMMIT;

-- Breakdown by primary tag key for verification
SELECT
    (SELECT key FROM jsonb_object_keys(tags) AS key
     ORDER BY key LIMIT 1) AS primary_key,
    count(*)
FROM feature
GROUP BY 1
ORDER BY 2 DESC;
