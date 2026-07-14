FROM debian:bookworm-slim AS tdlib
ARG TDLIB_GIT_REF=master
ARG TDLIB_BUILD_JOBS=2
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        cmake \
        git \
        gperf \
        libssl-dev \
        zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*
RUN git clone --depth 1 --branch "${TDLIB_GIT_REF}" https://github.com/tdlib/td.git /td
RUN cmake -S /td -B /td/build \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_INSTALL_PREFIX=/usr/local \
        -DTD_ENABLE_JNI=OFF \
    && cmake --build /td/build --target install -j"${TDLIB_BUILD_JOBS}"

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY --from=tdlib /usr/local /usr/local
RUN ldconfig
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -tags tdlib -o /out/api ./cmd/api

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libssl3 libstdc++6 wget zlib1g \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system app \
    && useradd --system --gid app --home-dir /nonexistent --shell /usr/sbin/nologin app \
    && mkdir -p /data/tdlib /data/exports /data/logs \
    && chown -R app:app /data
COPY --from=tdlib /usr/local/lib /usr/local/lib
RUN ldconfig
USER app
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
