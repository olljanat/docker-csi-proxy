FROM golang:1.20-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o csi-proxy cmd/csi-proxy/main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/csi-proxy /usr/bin/csi-proxy
COPY config.json /etc/docker/plugins/csi-proxy.json
ENTRYPOINT ["/usr/bin/csi-proxy"]
