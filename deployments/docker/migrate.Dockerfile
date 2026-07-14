FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/migrate ./cmd/migrate

FROM alpine:3.22
RUN addgroup -S app && adduser -S app -G app \
    && mkdir -p /data/logs \
    && chown -R app:app /data
USER app
WORKDIR /app
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY migrations ./migrations
ENTRYPOINT ["/usr/local/bin/migrate"]
