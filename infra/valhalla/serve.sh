#!/usr/bin/env bash
# Container PID 1: waits for tiles.tar built by valhalla-builder, then serves.
# Watches /tiles/reload sentinel written by valhalla-builder after each tile
# rebuild and restarts valhalla_service to pick up the new tiles.tar.
set -euo pipefail

TILES_TAR="/tiles/tiles.tar"
SERVE_CONFIG="/valhalla.json"
RELOAD_SENTINEL="/tiles/reload"
VALHALLA_PID=""

generate_serve_config() {
    echo "[valhalla] Generating serve config..."
    valhalla_build_config \
        --mjolnir-tile-dir /tiles \
        --mjolnir-timezone /tiles/tz_world.sqlite \
        --mjolnir-admin    /tiles/admins.sqlite \
        | python3 -c "
import json, sys
cfg = json.load(sys.stdin)
cfg['mjolnir']['tile_extract'] = '/tiles/tiles.tar'
cfg.get('mjolnir', {}).pop('traffic_extract', None)
# Mirror pedestrian service limits for scenic_pedestrian
sl = cfg.get('service_limits', {})
if 'pedestrian' in sl:
    sl['scenic_pedestrian'] = dict(sl['pedestrian'])
json.dump(cfg, sys.stdout)
" > "${SERVE_CONFIG}"
}

start_service() {
    echo "[valhalla] Starting valhalla_service..."
    valhalla_service "${SERVE_CONFIG}" 1 &
    VALHALLA_PID=$!
    echo "[valhalla] valhalla_service started (PID ${VALHALLA_PID})."
}

stop_service() {
    if [ -n "${VALHALLA_PID}" ] && kill -0 "${VALHALLA_PID}" 2>/dev/null; then
        echo "[valhalla] Stopping valhalla_service (PID ${VALHALLA_PID})..."
        kill -TERM "${VALHALLA_PID}"
        wait "${VALHALLA_PID}" || true
        VALHALLA_PID=""
    fi
}

# Propagate Docker stop to valhalla_service
trap 'stop_service; exit 0' TERM INT

# Wait for valhalla-builder to produce tiles.tar
echo "[valhalla] Waiting for ${TILES_TAR}..."
while [ ! -f "${TILES_TAR}" ]; do
    sleep 5 &
    wait $! || true  # returns immediately on SIGTERM
done
echo "[valhalla] tiles.tar found."

generate_serve_config
start_service

# Reload and crash-recovery loop
while true; do
    # Restart if valhalla_service crashed
    if [ -n "${VALHALLA_PID}" ] && ! kill -0 "${VALHALLA_PID}" 2>/dev/null; then
        echo "[valhalla] valhalla_service exited unexpectedly, restarting..."
        start_service
    fi

    if [ -f "${RELOAD_SENTINEL}" ]; then
        echo "[valhalla] Reload sentinel detected, reloading tiles..."
        rm -f "${RELOAD_SENTINEL}"
        stop_service
        start_service
    fi

    sleep 5 &
    wait $! || true
done
