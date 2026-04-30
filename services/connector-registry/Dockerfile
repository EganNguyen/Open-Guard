FROM golang:1.25-alpine AS builder

# Copy shared module
WORKDIR /shared
COPY shared/ ./

# Build service
WORKDIR /app
COPY services/connector-registry/go.mod services/connector-registry/go.sum ./
RUN go mod download
COPY services/connector-registry/ .
RUN go mod tidy && go build -o service .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/service /usr/local/bin/service
CMD ["service"]
