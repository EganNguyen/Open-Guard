FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go mod tidy && go build -o task-backend .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/task-backend /usr/local/bin/task-backend
EXPOSE 3005
CMD ["task-backend"]
