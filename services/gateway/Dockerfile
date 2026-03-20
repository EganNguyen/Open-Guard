FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY . .
WORKDIR /build/services/gateway
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /gateway .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /gateway /usr/local/bin/gateway

EXPOSE 8080
ENTRYPOINT ["gateway"]
