FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=docker" -o /app/oc-go-cc ./cmd/oc-go-cc

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && \
    mkdir -p /etc/oc-go-cc /root/.config/oc-go-cc

COPY --from=builder /app/oc-go-cc /usr/local/bin/oc-go-cc
COPY --from=builder /app/configs /tmp/configs
RUN if [ -f /tmp/configs/config.json ]; then \
      cp /tmp/configs/config.json /etc/oc-go-cc/config.json; \
    else \
      cp /tmp/configs/config.example.json /etc/oc-go-cc/config.json; \
    fi && \
    rm -rf /tmp/configs

ENV OC_GO_CC_CONFIG=/etc/oc-go-cc/config.json
ENV OC_GO_CC_HOST=0.0.0.0

EXPOSE 3456
ENTRYPOINT ["oc-go-cc", "serve"]
