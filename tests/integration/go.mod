module github.com/openguard/tests/integration

go 1.25.0

require (
	github.com/google/uuid v1.6.0
	github.com/openguard/sdk v0.0.0
	github.com/segmentio/kafka-go v0.4.51
	go.mongodb.org/mongo-driver v1.17.0
	github.com/jackc/pgx/v5 v5.9.2
)

replace github.com/openguard/sdk => ../../sdk
replace github.com/openguard/shared => ../../shared
