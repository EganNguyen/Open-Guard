FROM golang:alpine AS builder

WORKDIR /app
COPY go.mod ./
# Copy shared module if needed, but since we are using local paths or simple structure:
COPY . .
RUN go mod tidy && go build -o service .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/service /usr/local/bin/service
# Copy migrations so they can be run or referenced if needed
COPY migrations /app/migrations
CMD ["service"]
