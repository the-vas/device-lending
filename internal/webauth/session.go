package webauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

type Signer struct {
	secret []byte
}

func NewSigner(secret string) Signer {
	return Signer{secret: []byte(secret)}
}

func (s Signer) Sign(value string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(value))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(value)) + "." + sig
}

func (s Signer) Verify(signed string) (string, error) {
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		return "", errors.New("malformed signed value")
	}

	valueBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", errors.New("malformed signed value")
	}

	mac := hmac.New(sha256.New, s.secret)
	mac.Write(valueBytes)
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedSig), []byte(parts[1])) {
		return "", errors.New("invalid signature")
	}

	return string(valueBytes), nil
}
