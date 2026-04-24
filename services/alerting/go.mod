module github.com/openguard/services/alerting

go 1.25.0

replace github.com/openguard/shared => ../../shared

require (
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/openguard/shared v0.0.0-00010101000000-000000000000
	github.com/segmentio/kafka-go v0.4.51
	go.mongodb.org/mongo-driver v1.17.9
)

require github.com/stretchr/testify v1.8.3 // indirect

require (
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/klauspost/compress v1.16.7 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.16 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)
