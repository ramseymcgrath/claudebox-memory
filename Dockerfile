FROM golang:1.23-alpine AS builder

WORKDIR /build
COPY proxy/ .
RUN go mod download && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /memory-mcp-proxy .

FROM scratch
COPY --from=builder /memory-mcp-proxy /memory-mcp-proxy
ENTRYPOINT ["/memory-mcp-proxy"]
