FROM debian:bookworm-slim

# python3-osmium — provides pyosmium-up-to-date for PBF patching
# psycopg2-binary — PostgreSQL adapter
# numpy + requests — vector normalization and Ollama HTTP calls in embed_features.py
RUN apt-get update && apt-get install -y --no-install-recommends \
        osm2pgsql \
        osmium-tool \
        postgresql-client \
        lua5.4 \
        python3 \
        python3-pip \
        cron \
        curl \
    && pip3 install --break-system-packages osmium requests psycopg2-binary pgvector numpy \
    && rm -rf /var/lib/apt/lists/*

COPY download.sh import.sh init.sh update.sh /scripts/
COPY osm_tags.lua scenic.lua post-import.sql osm_tags.csv embed_features.py /scripts/
RUN chmod +x /scripts/download.sh /scripts/import.sh /scripts/init.sh /scripts/update.sh

# Daily update at 02:00 UTC: patch PBF + full PostGIS re-import
RUN echo "0 2 * * * root /scripts/update.sh >> /var/log/osm-update.log 2>&1" \
    > /etc/cron.d/osm-update && chmod 0644 /etc/cron.d/osm-update

VOLUME ["/data/osm"]

CMD ["cron", "-f"]
