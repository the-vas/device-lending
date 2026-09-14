package webauth_test

import (
	"testing"

	"github.com/the-vas/device-lending/internal/webauth"
)

func TestSigner_RoundTrip(t *testing.T) {
	s := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	signed := s.Sign("hello world")
	got, err := s.Verify(signed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestSigner_RejectsTamperedValue(t *testing.T) {
	s := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	signed := s.Sign("hello world")
	tampered := signed[:len(signed)-1] + "x"

	if _, err := s.Verify(tampered); err == nil {
		t.Fatal("expected error for tampered signature")
	}
}

func TestSigner_RejectsWrongSecret(t *testing.T) {
	s1 := webauth.NewSigner("secret-one-at-least-32-bytes-long")
	s2 := webauth.NewSigner("secret-two-at-least-32-bytes-long")

	signed := s1.Sign("hello world")
	if _, err := s2.Verify(signed); err == nil {
		t.Fatal("expected error when verifying with a different secret")
	}
}

func TestSigner_RejectsMalformedValue(t *testing.T) {
	s := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	if _, err := s.Verify("not-a-signed-value"); err == nil {
		t.Fatal("expected error for a malformed signed value")
	}
}
