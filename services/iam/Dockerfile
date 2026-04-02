FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY . .
WORKDIR /build/services/iam
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /iam .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /iam /usr/local/bin/iam

EXPOSE 8081
ENTRYPOINT ["iam"]
