FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .
RUN go mod tidy && go build -o service .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/service /usr/local/bin/service
COPY migrations /app/migrations
CMD ["service"]
