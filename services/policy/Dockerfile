FROM golang:1.24-alpine AS builder

WORKDIR /build
# Copy the entire workspace to support go.work
COPY . .

WORKDIR /build/services/policy
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /policy .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /policy .

EXPOSE 8082
ENTRYPOINT ["/app/policy"]
