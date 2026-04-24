module github.com/openguard/services/webhook-delivery

go 1.25.0

replace github.com/openguard/shared => ../../shared

require (
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/openguard/shared v0.0.0-00010101000000-000000000000
	github.com/segmentio/kafka-go v0.4.51
)

require github.com/stretchr/testify v1.8.3 // indirect

require (
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/klauspost/compress v1.15.11 // indirect
	github.com/pierrec/lz4/v4 v4.1.16 // indirect
	golang.org/x/crypto v0.50.0 // indirect
)
