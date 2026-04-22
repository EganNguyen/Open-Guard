FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod ./
# RUN go mod download (no go.sum yet)
COPY main.go ./
RUN go build -o service .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/service /usr/local/bin/service
CMD ["service"]
