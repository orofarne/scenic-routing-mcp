FROM postgis/postgis:17-3.5

# pgvector is available in the official PostgreSQL apt repo,
# which is already configured in the postgis/postgis base image.
RUN apt-get update \
    && apt-get install -y --no-install-recommends postgresql-17-pgvector \
    && rm -rf /var/lib/apt/lists/*
