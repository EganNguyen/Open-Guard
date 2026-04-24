package repository_test

import (
	"context"
	"testing"

	"github.com/openguard/services/policy/pkg/repository"
)

func TestWithOrgContext_MissingOrgID(t *testing.T) {
	// A nil pool is fine here because withOrgContext should return an error
	// before it even attempts to acquire a connection, due to the missing org_id.
	repo := repository.NewRepository(nil)

	_, err := repo.ListPolicies(context.Background(), "")
	if err == nil {
		t.Fatal("expected error due to missing org_id, got nil")
	}

	expectedErr := "withOrgContext: org_id missing from context"
	if err.Error() != expectedErr {
		t.Errorf("expected error %q, got: %v", expectedErr, err)
	}
}
