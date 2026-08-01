package oidcdiscovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Document struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

func Fetch(ctx context.Context, issuer string) (Document, error) {
	url := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Document{}, fmt.Errorf("building discovery request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Document{}, fmt.Errorf("fetching discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Document{}, fmt.Errorf("discovery endpoint returned status %d", resp.StatusCode)
	}

	var doc Document
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("parsing discovery document: %w", err)
	}

	if doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" || doc.UserinfoEndpoint == "" {
		return Document{}, fmt.Errorf("discovery document missing required endpoints")
	}

	return doc, nil
}
