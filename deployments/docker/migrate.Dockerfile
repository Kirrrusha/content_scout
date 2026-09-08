FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/migrate ./cmd/migrate

FROM alpine:3.22
ARG APP_UID=10001
ARG APP_GID=10001
RUN addgroup -S -g "$APP_GID" app && adduser -S -D -H -u "$APP_UID" -G app app \
    && mkdir -p /data/logs \
    && chown -R app:app /data
USER app
WORKDIR /app
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY migrations ./migrations
ENTRYPOINT ["/usr/local/bin/migrate"]
