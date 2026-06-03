# Builds Valhalla routing engine from source.
#
# Pin versions with build args.
# Rebuild this image to apply patches or upgrade Valhalla:
#
#   make valhalla-base
#   make valhalla-base VALHALLA_VERSION=3.8.0
ARG VALHALLA_VERSION=3.7.0
ARG PRIME_SERVER_VERSION=0.10.0

# ── Stage 1: compile ──────────────────────────────────────────────────────────
FROM ubuntu:24.04 AS build
ARG VALHALLA_VERSION
ARG PRIME_SERVER_VERSION

ENV DEBIAN_FRONTEND=noninteractive

# Build dependencies matching Valhalla's official install-linux-deps.sh
RUN apt-get update && apt-get install -y --no-install-recommends \
      autoconf \
      automake \
      build-essential \
      cmake \
      git \
      ca-certificates \
      pkg-config \
      libboost-all-dev \
      libcurl4-openssl-dev \
      libczmq-dev \
      libgdal-dev \
      libgeos++-dev \
      libgeos-dev \
      libluajit-5.1-dev \
      liblz4-dev \
      libprotobuf-dev \
      libspatialite-dev \
      libsqlite3-dev \
      libsqlite3-mod-spatialite \
      libtool \
      libzmq3-dev \
      luajit \
      python3-all-dev \
      python3-shapely \
      python3-requests \
      protobuf-compiler \
      zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

# Build prime_server (required by Valhalla ENABLE_SERVICES).
# Installed to /opt/prime_server to allow a clean COPY in the runtime stage.
RUN git clone \
      --branch "${PRIME_SERVER_VERSION}" \
      --depth 1 \
      --recurse-submodules \
      https://github.com/kevinkreiser/prime_server.git \
      /src/prime_server \
  && cd /src/prime_server \
  && ./autogen.sh \
  && ./configure --prefix=/usr/local \
  && make -j "$(nproc)" \
  && make install \
  && ldconfig

# Shallow-clone the exact tagged Valhalla commit with submodules.
RUN git clone \
      --branch "${VALHALLA_VERSION}" \
      --depth 1 \
      --recurse-submodules \
      --shallow-submodules \
      --jobs "$(nproc)" \
      https://github.com/valhalla/valhalla.git \
      /src/valhalla

# Apply scenic_pedestrian costing patch, path_factor extension, and steps TransitionCost discount.
COPY patches/scenic_pedestrian.patch patches/path_factor.patch patches/steps_transition.patch /src/valhalla-patches/
RUN patch -p1 -d /src/valhalla < /src/valhalla-patches/scenic_pedestrian.patch \
 && patch -p1 -d /src/valhalla < /src/valhalla-patches/path_factor.patch \
 && patch -p1 -d /src/valhalla < /src/valhalla-patches/steps_transition.patch

# Configure, build, install to /opt/valhalla.
RUN cmake -B /build -S /src/valhalla \
      -DCMAKE_BUILD_TYPE=Release \
      -DCMAKE_INSTALL_PREFIX=/opt/valhalla \
      -DENABLE_PYTHON_BINDINGS=OFF \
      -DENABLE_TESTS=OFF \
      -DENABLE_BENCHMARKS=OFF \
  && cmake --build /build -j "$(nproc)" \
  && cmake --install /build

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
FROM ubuntu:24.04 AS runtime
ARG VALHALLA_VERSION
ARG PRIME_SERVER_VERSION

ENV DEBIAN_FRONTEND=noninteractive

# Runtime dependencies (mirrors official Valhalla docker/Dockerfile runner stage)
RUN apt-get update && apt-get install -y --no-install-recommends \
      libboost-filesystem1.83.0 \
      libboost-regex1.83.0 \
      libboost-thread1.83.0 \
      libboost-program-options1.83.0 \
      libboost-iostreams1.83.0 \
      libboost-serialization1.83.0 \
      libboost-date-time1.83.0 \
      libcurl4 \
      libczmq4 \
      libluajit-5.1-2 \
      libgeotiff5 \
      libprotobuf-lite32 \
      libspatialite8t64 \
      libspatialite8t64 \
      libsqlite3-0 \
      libsqlite3-mod-spatialite \
      libzmq5 \
      zlib1g \
      python3-minimal \
      python-is-python3 \
      curl \
      ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Valhalla binaries and libraries
COPY --from=build /opt/valhalla /opt/valhalla

# prime_server shared library (installed to /usr/local in build stage)
COPY --from=build /usr/local/lib/libprime_server.so* /usr/local/lib/

RUN ldconfig

ENV PATH="/opt/valhalla/bin:${PATH}"
ENV LD_LIBRARY_PATH="/opt/valhalla/lib:/usr/local/lib${LD_LIBRARY_PATH:+:${LD_LIBRARY_PATH}}"

LABEL org.opencontainers.image.source="https://github.com/valhalla/valhalla"
LABEL org.opencontainers.image.version="${VALHALLA_VERSION}"
