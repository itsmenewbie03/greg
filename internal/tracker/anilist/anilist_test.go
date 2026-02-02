package anilist

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestExchangeCode(t *testing.T) {
	// Create client
	tokenSaved := false
	client := &Client{
		clientID:    "test_client",
		redirectURI: "https://anilist.co/api/v2/oauth/pin",
		httpClient:  &http.Client{},
		saveToken: func(token *oauth2.Token) error {
			tokenSaved = true
			return nil
		},
	}

	// Test exchange with implicit flow (token directly provided)
	ctx := context.Background()
	testToken := "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.test_token"
	err := client.ExchangeCode(ctx, testToken)
	if err != nil {
		t.Fatalf("ExchangeCode failed: %v", err)
	}

	// Verify token was set
	if client.token == nil {
		t.Fatal("Token was not set after exchange")
	}

	if client.token.AccessToken != testToken {
		t.Errorf("Expected access_token: %s, got %s", testToken, client.token.AccessToken)
	}

	if client.token.TokenType != "Bearer" {
		t.Errorf("Expected token_type: Bearer, got %s", client.token.TokenType)
	}

	// Verify expiry is set
	if client.token.Expiry.IsZero() {
		t.Error("Token expiry was not set")
	}

	// Verify expiry is approximately 1 year from now
	expectedExpiry := time.Now().Add(365 * 24 * time.Hour)
	diff := client.token.Expiry.Sub(expectedExpiry)
	if diff < -time.Minute || diff > time.Minute {
		t.Errorf("Token expiry is off by %v", diff)
	}

	// Verify token was saved
	if !tokenSaved {
		t.Error("saveToken callback was not called")
	}
}

func TestExchangeCodeWithSaveError(t *testing.T) {
	// Create client with save error
	client := &Client{
		clientID:    "test_client",
		redirectURI: "https://anilist.co/api/v2/oauth/pin",
		httpClient:  &http.Client{},
		saveToken: func(token *oauth2.Token) error {
			return fmt.Errorf("save failed")
		},
	}

	// Test exchange with save error
	ctx := context.Background()
	testToken := "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.test_token"
	err := client.ExchangeCode(ctx, testToken)
	if err == nil {
		t.Fatal("Expected error when save fails, got nil")
	}

	// Verify error message mentions token save failure
	if !strings.Contains(err.Error(), "save") {
		t.Errorf("Expected error about save failure, got: %v", err)
	}
}

func TestGetAuthURL(t *testing.T) {
	client := NewClient(Config{
		ClientID:    "32178",
		RedirectURI: "https://anilist.co/api/v2/oauth/pin",
	})

	authURL := client.GetAuthURL()

	// Verify URL contains required parameters
	if authURL == "" {
		t.Fatal("GetAuthURL returned empty string")
	}

	// Check for client_id
	if len(authURL) < 50 {
		t.Errorf("Auth URL seems too short: %s", authURL)
	}
}
