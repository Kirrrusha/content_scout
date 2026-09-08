FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/bot ./cmd/bot

FROM debian:bookworm-slim
ARG APP_UID=10001
ARG APP_GID=10001
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid "$APP_GID" app \
    && useradd --uid "$APP_UID" --gid app --home-dir /nonexistent --shell /usr/sbin/nologin --no-create-home app \
    && mkdir -p /data/exports /data/logs \
    && chown -R app:app /data
USER app
COPY --from=build /out/bot /usr/local/bin/bot
ENTRYPOINT ["/usr/local/bin/bot"]
