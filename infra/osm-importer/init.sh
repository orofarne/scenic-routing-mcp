#!/usr/bin/env bash
# One-shot startup init: download PBF if missing, then populate the DB if empty.
# Runs as osm-importer-init before valhalla and osm-importer start.
set -euo pipefail

PBF_FILE="${OSM_PBF_FILE:-/data/osm/region.osm.pbf}"
DB_URL="postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@db:5432/${POSTGRES_DB}"

# Step 1: download PBF(s) if the working PBF is not present.
# For multi-region setups, download.sh also downloads individual region files
# and merges them; the sentinel is the merged file at $OSM_PBF_FILE.
if [ -f "${PBF_FILE}" ]; then
    echo "[osm-importer-init] PBF already present, skipping download."
else
    echo "[osm-importer-init] PBF not found, downloading..."
    /scripts/download.sh
fi

# Step 2: initial PostGIS import if the feature table is empty
DB_POPULATED=$(psql "${DB_URL}" -t -c "SELECT EXISTS (SELECT 1 FROM feature LIMIT 1);" | tr -d ' \n')
if [ "${DB_POPULATED}" = "t" ]; then
    echo "[osm-importer-init] Database already populated, skipping import."
    # embed_features.py is idempotent (WHERE poi_vec IS NULL) — run it to recover
    # from an interrupted previous run or a fresh deploy on an existing volume.
    echo "[osm-importer-init] Running embedding for pending features..."
    python3 /scripts/embed_features.py
else
    echo "[osm-importer-init] Database is empty, running initial import..."
    /scripts/import.sh
fi

echo "[osm-importer-init] Done."
