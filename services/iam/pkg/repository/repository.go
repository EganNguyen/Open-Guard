package repository

// Repository is the canonical implementation of the IAM repository.
// It consolidates all entity-specific repository methods (Org, User, Session, etc.)
// into a single struct per the OpenGuard System Specification v2.0 (§0.3).
type Repository struct{}

// New creates a new instance of the Repository.
func New() *Repository {
	return &Repository{}
}
