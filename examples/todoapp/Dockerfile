# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod (no go.sum as there are no external dependencies)
COPY go.mod ./
RUN go mod download

# Copy source code and HTML
COPY main.go ./
COPY index.html ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o todoapp main.go

# Final stage
FROM alpine:latest

WORKDIR /app

# Copy the binary and HTML from the builder stage
COPY --from=builder /app/todoapp .
COPY --from=builder /app/index.html .

# Expose the application port
EXPOSE 8082

# Run the application
CMD ["./todoapp"]
