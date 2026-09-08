ARG TDLIB_BASE_IMAGE=kirrrusha.cr.cloud.ru/content_scout-tdlib-base:latest
FROM ${TDLIB_BASE_IMAGE} AS tdlib

FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY --from=tdlib /usr/local /usr/local
RUN ldconfig
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -tags tdlib -o /out/tdlib-worker ./cmd/tdlib-worker

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libssl3 libstdc++6 zlib1g \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system app \
    && useradd --system --gid app --home-dir /nonexistent --shell /usr/sbin/nologin app \
    && mkdir -p /data/tdlib /data/logs \
    && chown -R app:app /data
COPY --from=tdlib /usr/local/lib /usr/local/lib
RUN ldconfig
USER app
COPY --from=build /out/tdlib-worker /usr/local/bin/tdlib-worker
ENTRYPOINT ["/usr/local/bin/tdlib-worker"]
