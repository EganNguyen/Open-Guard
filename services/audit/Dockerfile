FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY . .
WORKDIR /build/services/audit
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /audit .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /audit /usr/local/bin/audit

EXPOSE 8083
ENTRYPOINT ["audit"]
