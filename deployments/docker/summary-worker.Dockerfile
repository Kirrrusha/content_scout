FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/summary-worker ./cmd/summary-worker

FROM alpine:3.22
ARG APP_UID=10001
ARG APP_GID=10001
RUN addgroup -S -g "$APP_GID" app && adduser -S -D -H -u "$APP_UID" -G app app \
    && mkdir -p /data/exports /data/logs \
    && chown -R app:app /data
USER app
COPY --from=build /out/summary-worker /usr/local/bin/summary-worker
ENTRYPOINT ["/usr/local/bin/summary-worker"]
