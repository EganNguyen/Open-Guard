module github.com/openguard/examples/task-management-app/backend

go 1.25.0

require (
	github.com/go-chi/chi/v5 v5.0.10
	github.com/jackc/pgx/v5 v5.4.3
	github.com/openguard/sdk v0.0.0
	github.com/openguard/shared v0.0.0
)

replace github.com/openguard/sdk => ../../../sdk
replace github.com/openguard/shared => ../../../shared
