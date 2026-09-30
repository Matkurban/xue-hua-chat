FROM golang:1.27.1-alpine AS builder
WORKDIR /chat
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN set -eu; mkdir -p /chat/bin; \
    for source in cmd/api/chat-api-*/main.go cmd/rpc/chat-rpc-*/main.go tools/check-component/main.go tools/attribute-to-credential/main.go; do \
      directory="${source%/main.go}"; binary="${directory##*/}"; \
      CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "/chat/bin/${binary}" "./${directory}"; \
    done

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /chat
COPY --from=builder /chat/bin ./bin
COPY --from=builder /chat/config ./config
COPY --from=builder /chat/LICENSE ./LICENSE
ENV PATH="/chat/bin:${PATH}"
CMD ["/chat/bin/chat-api-chat", "-c", "/chat/config"]
