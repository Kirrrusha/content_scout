FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/summary-worker ./cmd/summary-worker

FROM alpine:3.22
RUN addgroup -S app && adduser -S app -G app \
    && mkdir -p /data/exports /data/logs \
    && chown -R app:app /data
USER app
COPY --from=build /out/summary-worker /usr/local/bin/summary-worker
ENTRYPOINT ["/usr/local/bin/summary-worker"]
