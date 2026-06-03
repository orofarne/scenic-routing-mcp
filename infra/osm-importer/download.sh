#!/usr/bin/env bash
# Downloads OSM PBF extract(s) and produces the working PBF at $OSM_PBF_FILE.
#
# Single region: downloads directly to $OSM_PBF_FILE — no merge step, no osmium needed.
# Multiple regions: downloads each to /data/osm/<basename>, then merges with osmium.
#
# Region source:
#   OSM_REGIONS — semicolon-separated URLs: "https://.../a-latest.osm.pbf;https://.../b-latest.osm.pbf"
set -euo pipefail

DEST="${OSM_PBF_FILE:-/data/osm/region.osm.pbf}"
REGIONS="${OSM_REGIONS:-https://download.geofabrik.de/europe/united-kingdom/england/greater-london-latest.osm.pbf}"

_validate_pbf() {
    local path="$1"
    python3 - "$path" <<'PYEOF'
import struct, sys
path = sys.argv[1]
with open(path, 'rb') as f:
    data = f.read(4)
if len(data) < 4:
    sys.exit(1)
size = struct.unpack('>I', data)[0]
if size > 65536:
    print(f"[download] ERROR: invalid PBF (blob-header-size={size} — likely HTML error page was saved)", file=sys.stderr)
    sys.exit(1)
PYEOF
}

_download_one() {
    local url="$1"
    local dest="$2"
    mkdir -p "$(dirname "$dest")"
    echo "[download] Fetching: $url"
    curl -L --fail --retry 3 --progress-bar -o "$dest" "$url"
    echo "[download] Validating PBF structure..."
    if ! _validate_pbf "$dest"; then
        rm -f "$dest"
        exit 1
    fi
    echo "[download] OK: $dest ($(du -sh "$dest" | cut -f1))"
}

IFS=';' read -ra URLS <<< "$REGIONS"
COUNT=${#URLS[@]}

if [ "$COUNT" -eq 1 ]; then
    # Single region: download directly to DEST — identical behaviour to original script.
    _download_one "${URLS[0]}" "$DEST"
else
    # Multiple regions: download each to a named file, then merge into DEST.
    INDIVIDUAL=()
    for url in "${URLS[@]}"; do
        local_file="/data/osm/$(basename "$url")"
        _download_one "$url" "$local_file"
        INDIVIDUAL+=("$local_file")
    done
    echo "[download] Merging ${COUNT} regions into ${DEST}..."
    osmium merge "${INDIVIDUAL[@]}" -o "$DEST" --overwrite
    echo "[download] Merge complete: $DEST ($(du -sh "$DEST" | cut -f1))"
fi
