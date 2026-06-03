#!/usr/bin/env bash
# Daily update (runs at 02:00 UTC via cron).
#
# Single source of truth: pyosmium patches the PBF(s), then both consumers rebuild from it:
#   - osm2pgsql full re-import  → PostGIS feature table
#   - Valhalla (03:00 UTC cron) → routing tiles rebuilt from the same PBF
#
# Single region: patches $OSM_PBF_FILE directly.
# Multiple regions: patches each /data/osm/<basename> independently
#   (each file carries its own replication state), then merges into $OSM_PBF_FILE.
set -euo pipefail

MERGED_PBF="${OSM_PBF_FILE:-/data/osm/region.osm.pbf}"
DB_URL="postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@db:5432/${POSTGRES_DB}"
REGIONS="${OSM_REGIONS:-https://download.geofabrik.de/europe/united-kingdom/england/greater-london-latest.osm.pbf}"

IFS=';' read -ra URLS <<< "$REGIONS"
COUNT=${#URLS[@]}

echo "[update] $(date -u +%Y-%m-%dT%H:%M:%SZ) Updating PBF(s)..."

if [ "$COUNT" -eq 1 ]; then
    # Single region: patch the working PBF directly.
    # Replication URL is embedded in the PBF header by Geofabrik.
    pyosmium-up-to-date --size 2000 "$MERGED_PBF"
else
    # Multiple regions: patch each file independently, then re-merge.
    INDIVIDUAL=()
    for url in "${URLS[@]}"; do
        file="/data/osm/$(basename "$url")"
        echo "[update] Updating ${file}..."
        pyosmium-up-to-date --size 2000 "$file"
        INDIVIDUAL+=("$file")
    done
    echo "[update] Merging ${COUNT} regions into ${MERGED_PBF}..."
    osmium merge "${INDIVIDUAL[@]}" -o "$MERGED_PBF" --overwrite
fi

# Signal valhalla-builder immediately after PBF is ready so tile rebuild runs
# in parallel with the PostGIS import below (both read the same PBF).
touch "${MERGED_PBF%/*}/pbf_updated"

echo "[update] Re-importing into PostGIS..."
osm2pgsql \
    --output=flex \
    --style=/scripts/scenic.lua \
    --database="${DB_URL}" \
    --slim \
    --drop \
    --cache=512 \
    --number-processes=4 \
    "${MERGED_PBF}"

echo "[update] Running post-import SQL..."
psql --set ON_ERROR_STOP=1 "${DB_URL}" -f /scripts/post-import.sql

echo "[update] Computing POI embeddings..."
python3 /scripts/embed_features.py

echo "[update] $(date -u +%Y-%m-%dT%H:%M:%SZ) Done."
