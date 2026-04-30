FROM golang:1.25-alpine AS builder

# Copy shared module
WORKDIR /shared
COPY shared/ ./

# Build service
WORKDIR /app
COPY services/webhook-delivery/go.mod services/webhook-delivery/go.sum ./
RUN go mod download
COPY services/webhook-delivery/ .
RUN go mod tidy && go build -o service .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/service /usr/local/bin/service
CMD ["service"]
