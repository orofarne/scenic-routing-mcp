#!/usr/bin/env bash
# Builds Valhalla routing tiles and watches for PBF updates.
#
# Startup: builds tiles immediately if tiles.tar does not yet exist.
# Loop:    polls every 30s for /data/osm/pbf_updated sentinel written by
#          osm-importer after each nightly PBF refresh.
# On trigger: rebuilds tiles to a temp dir, packages atomically into
#             tiles.tar, then writes /tiles/reload for valhalla to pick up.
set -euo pipefail

PBF="${OSM_PBF_FILE:-/data/osm/region.osm.pbf}"
TILES_TAR="/tiles/tiles.tar"
TILES_TMP="/tiles/build_tmp"
BUILD_CONFIG="/valhalla_build.json"
RELOAD_SENTINEL="/tiles/reload"
PBF_UPDATED_SENTINEL="/data/osm/pbf_updated"

generate_build_config() {
    valhalla_build_config \
        --mjolnir-tile-dir      "${TILES_TMP}" \
        --mjolnir-timezone      "/tiles/tz_world.sqlite" \
        --mjolnir-admin         "/tiles/admins.sqlite" \
        | python3 -c "
import json, sys
cfg = json.load(sys.stdin)
cfg.get('mjolnir', {}).pop('tile_extract', None)
cfg.get('mjolnir', {}).pop('traffic_extract', None)
json.dump(cfg, sys.stdout)
" > "${BUILD_CONFIG}"
}

build_tiles() {
    echo "[valhalla-builder] $(date -u +%Y-%m-%dT%H:%M:%SZ) Building tiles from ${PBF}..."
    rm -rf "${TILES_TMP}"
    mkdir -p "${TILES_TMP}"

    generate_build_config
    valhalla_build_tiles -c "${BUILD_CONFIG}" "${PBF}"

    # admin/tz sqlite files are built to /tiles/ directly (fixed path for serve config)
    echo "[valhalla-builder] Packaging tiles into tar..."
    # Exclude sqlite files — they live at fixed paths and are already in /tiles/
    tar -cf "${TILES_TAR}.tmp" \
        --exclude="admins.sqlite" \
        --exclude="tz_world.sqlite" \
        -C "${TILES_TMP}" .
    # Atomic rename: valhalla will see either old or new tar, never a partial file
    mv "${TILES_TAR}.tmp" "${TILES_TAR}"

    rm -rf "${TILES_TMP}"
    echo "[valhalla-builder] $(date -u +%Y-%m-%dT%H:%M:%SZ) tiles.tar ready."
}

# Initial build if no tiles.tar exists yet.
# No reload sentinel here: valhalla/serve.sh waits for tiles.tar to appear,
# so it will pick up the fresh tar directly without needing a reload signal.
if [ ! -f "${TILES_TAR}" ]; then
    echo "[valhalla-builder] No tiles.tar found, running initial build..."
    build_tiles
fi

echo "[valhalla-builder] Watching for PBF updates (sentinel: ${PBF_UPDATED_SENTINEL})..."
while true; do
    if [ -f "${PBF_UPDATED_SENTINEL}" ]; then
        echo "[valhalla-builder] PBF update detected, rebuilding tiles..."
        rm -f "${PBF_UPDATED_SENTINEL}"
        build_tiles
        touch "${RELOAD_SENTINEL}"
        echo "[valhalla-builder] Reload sentinel written."
    fi
    sleep 30
done
