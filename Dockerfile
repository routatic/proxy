FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /app/routatic-proxy ./cmd/routatic-proxy

FROM alpine:3.21
LABEL org.opencontainers.image.source="https://github.com/routatic/proxy"
LABEL org.opencontainers.image.description="Proxy Claude Code requests to OpenCode Go API"
LABEL org.opencontainers.image.licenses="AGPL-3.0-only"
RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S appgroup && adduser -S appuser -G appgroup && \
    mkdir -p /etc/routatic-proxy && \
    chown -R appuser:appgroup /etc/routatic-proxy

COPY --from=builder /app/routatic-proxy /usr/local/bin/routatic-proxy
RUN ln -s /usr/local/bin/routatic-proxy /usr/local/bin/oc-go-cc
COPY --from=builder /app/configs/config.example.json /etc/routatic-proxy/config.json
RUN chown -R appuser:appgroup /etc/routatic-proxy

USER appuser

ENV ROUTATIC_PROXY_CONFIG=/etc/routatic-proxy/config.json
ENV ROUTATIC_PROXY_HOST=0.0.0.0

EXPOSE 3456

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:3456/health || exit 1

ENTRYPOINT ["routatic-proxy", "start", "--headless"]
