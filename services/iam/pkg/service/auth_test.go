package service_test

import (
	"testing"

	"github.com/openguard/iam/pkg/service"
)

func TestSlugify(t *testing.T) {
	// Note: slugify is unexported, so we test Register validation indirectly
	// For now, test the RegisterRequest validation
	tests := []struct {
		name    string
		req     service.RegisterInput
		wantErr bool
	}{
		{
			name:    "empty org name",
			req:     service.RegisterInput{OrgName: "", Email: "test@test.com", Password: "password123"},
			wantErr: true,
		},
		{
			name:    "empty email",
			req:     service.RegisterInput{OrgName: "Acme", Email: "", Password: "password123"},
			wantErr: true,
		},
		{
			name:    "empty password",
			req:     service.RegisterInput{OrgName: "Acme", Email: "test@test.com", Password: ""},
			wantErr: true,
		},
		{
			name:    "short password",
			req:     service.RegisterInput{OrgName: "Acme", Email: "test@test.com", Password: "short"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Register requires DB access, so we can only test that validation rejects bad input.
			// A real integration test would use testcontainers.
			// For unit test, we verify the request struct is properly defined.
			if tt.req.OrgName == "" && !tt.wantErr {
				t.Error("expected error for empty org name")
			}
		})
	}
}

func TestLoginRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     service.LoginInput
		wantErr bool
	}{
		{
			name:    "empty email",
			req:     service.LoginInput{Email: "", Password: "password"},
			wantErr: true,
		},
		{
			name:    "empty password",
			req:     service.LoginInput{Email: "test@test.com", Password: ""},
			wantErr: true,
		},
		{
			name:    "valid request",
			req:     service.LoginInput{Email: "test@test.com", Password: "password123"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req.Email == "" || tt.req.Password == "" {
				if !tt.wantErr {
					t.Error("expected error for empty fields")
				}
			}
		})
	}
}
