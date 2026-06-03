#!/usr/bin/env bash
# Full initial import of an OSM PBF file into PostGIS.
# Usage: /scripts/import.sh [/data/osm/region.osm.pbf]
set -euo pipefail

PBF_FILE="${1:-${OSM_PBF_FILE:-/data/osm/region.osm.pbf}}"
DB_URL="postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@db:5432/${POSTGRES_DB}"

if [[ ! -f "${PBF_FILE}" ]]; then
    echo "[import] ERROR: PBF file not found: ${PBF_FILE}" >&2
    echo "[import] Run: docker compose run --rm osm-importer /scripts/download.sh" >&2
    exit 1
fi

echo "[import] Waiting for PostgreSQL..."
until pg_isready -h db -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -q; do
    sleep 2
done

echo "[import] Starting import: ${PBF_FILE}"
# --slim --drop: slim tables are needed for correct multi-pass relation processing;
# --drop removes them afterwards since subsequent updates re-create them from scratch.
osm2pgsql \
    --output=flex \
    --style=/scripts/scenic.lua \
    --database="${DB_URL}" \
    --slim \
    --drop \
    --cache=512 \
    --number-processes=4 \
    "${PBF_FILE}"

echo "[import] Running post-import SQL..."
psql --set ON_ERROR_STOP=1 "${DB_URL}" -f /scripts/post-import.sql

echo "[import] Computing POI embeddings..."
python3 /scripts/embed_features.py

echo "[import] Done. Run 'docker compose up -d' to start all services."
