module github.com/openguard/services/threat

go 1.25.0

replace github.com/openguard/shared => ../../shared

require (
	github.com/redis/go-redis/v9 v9.18.0
	github.com/segmentio/kafka-go v0.4.51
)

require github.com/stretchr/testify v1.8.3 // indirect

require (
	github.com/alicebob/miniredis/v2 v2.37.0
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)
