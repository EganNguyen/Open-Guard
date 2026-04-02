FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY . .
WORKDIR /build/services/controlplane
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /controlplane .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /controlplane /usr/local/bin/controlplane

EXPOSE 8080
ENTRYPOINT ["controlplane"]
