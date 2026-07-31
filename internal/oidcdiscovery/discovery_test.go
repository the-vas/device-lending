package oidcdiscovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"authorization_endpoint": "https://auth.example.com/authorize",
			"token_endpoint": "https://auth.example.com/token",
			"userinfo_endpoint": "https://auth.example.com/userinfo"
		}`))
	}))
	defer srv.Close()

	doc, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.AuthorizationEndpoint != "https://auth.example.com/authorize" {
		t.Errorf("unexpected AuthorizationEndpoint: %s", doc.AuthorizationEndpoint)
	}
	if doc.TokenEndpoint != "https://auth.example.com/token" {
		t.Errorf("unexpected TokenEndpoint: %s", doc.TokenEndpoint)
	}
	if doc.UserinfoEndpoint != "https://auth.example.com/userinfo" {
		t.Errorf("unexpected UserinfoEndpoint: %s", doc.UserinfoEndpoint)
	}
}

func TestFetch_MissingEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for a discovery document missing required endpoints")
	}
}

func TestFetch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error on non-200 response")
	}
}
