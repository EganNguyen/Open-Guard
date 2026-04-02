package handlers

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"
)

type AuthClient interface {
	AuthURL(state string) string
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
}

type AuthHandler struct {
	client      AuthClient
	frontendURL string
}

func NewAuthHandler(client AuthClient, frontendURL string) *AuthHandler {
	return &AuthHandler{client: client, frontendURL: frontendURL}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	state := "random-state" // TODO: use secure state and CSRF
	url := h.client.AuthURL(state)
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing_code", http.StatusBadRequest)
		return
	}

	token, err := h.client.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "exchange_failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back to frontend with tokens in URL (or Set-Cookie)
	// Spec says: "redirect with auth code ... exchange ... { access_token, refresh_token }"
	url := h.frontendURL + "?access_token=" + token.AccessToken
	if refreshToken, ok := token.Extra("refresh_token").(string); ok {
		url += "&refresh_token=" + refreshToken
	}
	
	http.Redirect(w, r, url, http.StatusFound)
}
