#!/bin/sh
set -e

TILES_URL="${MAP_TILES_URL:-https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png}"
TILES_ATTR="${MAP_TILES_ATTR:-&copy; <a href=\"https://www.openstreetmap.org/copyright\">OpenStreetMap</a> contributors}"

# Escape & for sed replacement — in sed's replacement string & means "the full match".
TILES_URL_SAFE=$(printf '%s\n' "$TILES_URL" | sed 's/&/\\&/g')
TILES_ATTR_SAFE=$(printf '%s\n' "$TILES_ATTR" | sed 's/&/\\&/g')

sed \
  -e "s|__MAP_TILES_URL__|${TILES_URL_SAFE}|g" \
  -e "s|__MAP_TILES_ATTR__|${TILES_ATTR_SAFE}|g" \
  /usr/share/nginx/html/index.html.template \
  > /usr/share/nginx/html/index.html

exec nginx -g 'daemon off;'
