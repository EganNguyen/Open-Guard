FROM golang:1.25-alpine AS builder

# Copy shared module
WORKDIR /shared
COPY shared/ ./

# Build service
WORKDIR /app
COPY services/threat/go.mod ./
COPY services/threat/ .
RUN go mod tidy && go build -o service .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/service /usr/local/bin/service
CMD ["service"]
