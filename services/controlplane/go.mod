module github.com/openguard/controlplane

go 1.25.0

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/go-chi/cors v1.2.2
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/openguard/shared v0.0.0
	github.com/redis/go-redis/v9 v9.7.0
	github.com/sony/gobreaker v1.0.0
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.9.1 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

replace github.com/openguard/shared => ../../shared
