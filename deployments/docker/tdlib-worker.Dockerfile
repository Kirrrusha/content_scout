FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/tdlib-worker ./cmd/tdlib-worker

FROM alpine:3.22
RUN addgroup -S app && adduser -S app -G app
USER app
COPY --from=build /out/tdlib-worker /usr/local/bin/tdlib-worker
ENTRYPOINT ["/usr/local/bin/tdlib-worker"]
