DICTIONARY        := internal/dictionary/osm_tags.csv
_EXTRA_COMPOSE    := $(patsubst %,-f %,$(wildcard ./docker-compose.d/*.yaml ./docker-compose.d/*.yml))
COMPOSE           := docker compose -f docker-compose.yml $(_EXTRA_COMPOSE)

VALHALLA_VERSION      := 3.7.0
PRIME_SERVER_VERSION  := 0.10.0
VALHALLA_BASE_IMG     := scenic-routing-mcp/valhalla-base:$(VALHALLA_VERSION)

.PHONY: dictionary lint test cover check valhalla-base up down clean

# Rebuild the OSM tag description dictionary from taginfo + OSM Wiki.
# Output is written to $(DICTIONARY); rows with empty descriptions need manual review.
dictionary: $(DICTIONARY)

$(DICTIONARY): internal/dictionary/osm_tags.csv

internal/dictionary/osm_tags.csv: infra/osm-importer/osm_tags.csv
	cp $< $@

infra/osm-importer/osm_tags.csv:
	go run ./cmd/build-dictionary/ -o $@

# Run golangci-lint on all Go packages.
lint:
	golangci-lint run ./...

# Run all unit tests.
test:
	go test ./...

# Show per-function test coverage across all packages.
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Run lint and tests together.
check: lint test

# Build (or rebuild) the Valhalla base image from source.
# Run this once before 'make up', and whenever you change VALHALLA_VERSION or apply patches.
# First build takes ~20 min (native arm64) or longer under Rosetta (amd64 emulation).
valhalla-base:
	docker build \
	  --build-arg VALHALLA_VERSION=$(VALHALLA_VERSION) \
	  --build-arg PRIME_SERVER_VERSION=$(PRIME_SERVER_VERSION) \
	  -t $(VALHALLA_BASE_IMG) \
	  ./infra/valhalla-base

# Start all services (db, osm-importer, valhalla, mcp).
# Automatically builds the valhalla-base image if it is not present locally.
up:
	@if ! docker image inspect $(VALHALLA_BASE_IMG) > /dev/null 2>&1; then \
	  echo "valhalla-base image not found — building from source (first run, ~20 min)..."; \
	  $(MAKE) valhalla-base; \
	fi
	$(COMPOSE) up -d --build

# Stop all services (keeps named volumes intact).
down:
	$(COMPOSE) down

# Stop all services and remove named volumes (pgdata, osm-data, valhalla-tiles, ollama-data).
# Use for a full reset — next 'make up' will re-download OSM data and re-import.
clean:
	$(COMPOSE) down -v
