FROM golang:1.25-alpine AS builder

# Copy shared and sdk modules
WORKDIR /shared
COPY shared/ ./
WORKDIR /sdk
COPY sdk/ ./

# Build service
WORKDIR /app
COPY examples/task-management-app/backend/go.mod ./
# No go.sum yet, or copy it if it exists
COPY examples/task-management-app/backend/ .
RUN go mod tidy && go build -o service .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/service /usr/local/bin/service
CMD ["service"]
