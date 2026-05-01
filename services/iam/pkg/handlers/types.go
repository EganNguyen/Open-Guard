package handlers

import iam_repo "github.com/openguard/services/iam/pkg/repository"

type loginResponse struct {
	User         *iam_repo.User `json:"user"`
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	ExpiresIn    int            `json:"expires_in,omitempty"`
}

type mfaChallengeResponse struct {
	MFARequired  bool   `json:"mfa_required"`
	MFAChallenge string `json:"mfa_challenge"`
}

type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

type genericResponse struct {
	Message string `json:"message,omitempty"`
	Status  string `json:"status,omitempty"`
	ID      string `json:"id,omitempty"`
	Code    string `json:"code,omitempty"`
}

type webAuthnBeginResponse struct {
	SessionID string      `json:"session_id"`
	Options   interface{} `json:"options"`
}
