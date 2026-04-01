package service

import (
	"context"
	"errors"
	"testing"

	"github.com/openguard/policy/pkg/repository"
)

func TestPolicyService_Get(t *testing.T) {
	svc := New(&mockRepo{}, nil, 0, nil)
	_, err := svc.Get(context.Background(), "o1", "p1")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected repository.ErrNotFound, got %v", err)
	}
}

func TestPolicyService_List(t *testing.T) {
	svc := New(&mockRepo{}, nil, 0, nil)
	policies, err := svc.List(context.Background(), "o1")
	if err != nil {
		t.Errorf("unexpected err: %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("expected empty slices")
	}
}

func TestPolicyService_Delete(t *testing.T) {
	svc := New(&mockRepo{}, nil, 0, nil)
	err := svc.Delete(context.Background(), "o1", "p1")
	if err != nil {
		t.Errorf("unexpected err: %v", err)
	}
}
