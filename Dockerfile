FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /unraid-mcp ./cmd/server

FROM alpine:3.21

LABEL org.opencontainers.image.title="unraid-mcp" \
      org.opencontainers.image.description="MCP server for Unraid management" \
      org.opencontainers.image.source="https://github.com/jamesprial/unraid-mcp"

RUN apk add --no-cache ca-certificates tini

# Unraid standard: nobody(99):users(100)
RUN addgroup -g 100 -S users 2>/dev/null || true && \
    adduser -D -u 99 -G users mcp

RUN mkdir -p /config && chown 99:100 /config

COPY --from=builder /unraid-mcp /usr/local/bin/unraid-mcp

USER 99:100
EXPOSE 8080
VOLUME ["/config"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/mcp || exit 1

ENTRYPOINT ["tini", "--"]
CMD ["unraid-mcp"]
