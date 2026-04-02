# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and HTML
COPY main.go ./
COPY index.html ./
COPY pkg/ ./pkg/

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o todoapp main.go

# Final stage
FROM alpine:latest

WORKDIR /app

# Copy the binary and HTML from the builder stage
COPY --from=builder /app/todoapp .
COPY --from=builder /app/index.html .

# Expose the application port
EXPOSE 8083

# Run the application
CMD ["./todoapp"]
