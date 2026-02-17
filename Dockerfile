FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /unraid-mcp ./cmd/server

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /unraid-mcp /usr/local/bin/unraid-mcp
EXPOSE 8080
ENTRYPOINT ["unraid-mcp"]
