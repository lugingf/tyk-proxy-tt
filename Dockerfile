FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY . .

RUN go build -o /app/tyk_proxy ./cmd/tyk-proxy

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/tyk_proxy /app/tyk_proxy
EXPOSE 8080

#CMD ["/app/tyk_proxy", "-config", "/app/config.json", "-env"]
CMD ["/app/tyk_proxy", "-config", "/app/config.json"]
