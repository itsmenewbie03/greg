package anilist

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
)

const (
	// OAuth URL for AniList (this is fixed and won't change)
	browserAuthOAuthURL = "https://anilist.co/api/v2/oauth"
)

// BrowserAuthConfig holds configuration for browser-based OAuth authentication
type BrowserAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	ServerPort   int
}

// BrowserAuthResult contains the result of browser authentication
type BrowserAuthResult struct {
	Token    *oauth2.Token
	Username string
	UserID   int
}

// AuthenticateWithBrowser performs OAuth authentication using the browser.
// It starts a local server to handle the OAuth callback and opens the browser
// for the user to authenticate with AniList.
func AuthenticateWithBrowser(ctx context.Context, authConfig BrowserAuthConfig, saveToken func(*oauth2.Token) error) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Start local server to handle OAuth callback
	callbackCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", authConfig.ServerPort),
		Handler: mux,
	}

	// Handle OAuth callback - authorization code comes in query params
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		errorParam := r.URL.Query().Get("error")

		w.Header().Set("Content-Type", "text/html")

		if errorParam != "" {
			w.WriteHeader(http.StatusBadRequest)
			html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Greg Authentication</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 50px; text-align: center; background: #1a1a1a; color: white; }
        .error { color: #f44336; font-size: 18px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="error">Authentication failed: %s</div>
    <p>You can close this window and try again.</p>
</body>
</html>`, errorParam)
			_, _ = fmt.Fprint(w, html)
			errCh <- fmt.Errorf("oauth error: %s", errorParam)
			return
		}

		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			html := `<!DOCTYPE html>
<html>
<head>
    <title>Greg Authentication</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 50px; text-align: center; background: #1a1a1a; color: white; }
        .error { color: #f44336; font-size: 18px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="error">No authorization code received</div>
    <p>You can close this window and try again.</p>
</body>
</html>`
			_, _ = fmt.Fprint(w, html)
			errCh <- fmt.Errorf("no authorization code received")
			return
		}

		// Exchange authorization code for access token in background
		go func() {
			token, err := exchangeCodeForToken(code, authConfig)
			if err != nil {
				errCh <- err
				return
			}
			callbackCh <- token
		}()

		// Show success page immediately
		html := `<!DOCTYPE html>
<html>
<head>
    <title>Greg Authentication</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 50px; text-align: center; background: #1a1a1a; color: white; }
        .success { color: #4CAF50; font-size: 18px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="success">Authentication successful!</div>
    <p>You can close this window and return to the terminal.</p>
</body>
</html>`
		_, _ = fmt.Fprint(w, html)
	})

	// Start server in background
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("failed to start server: %w", err)
		}
	}()
	defer func() { _ = srv.Shutdown(ctx) }()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Open browser for authentication using Authorization Code Grant flow
	authURL := fmt.Sprintf("%s/authorize?client_id=%s&redirect_uri=%s&response_type=code",
		browserAuthOAuthURL,
		authConfig.ClientID,
		url.QueryEscape(authConfig.RedirectURI))

	fmt.Println("Opening browser for AniList authentication...")
	fmt.Printf("If the browser doesn't open automatically, visit: %s\n", authURL)

	if err := browser.OpenURL(authURL); err != nil {
		fmt.Printf("Failed to open browser automatically: %v\n", err)
		fmt.Println("Please copy and paste the URL above into your browser")
	}

	// Wait for token
	var accessToken string
	select {
	case accessToken = <-callbackCh:
	case err := <-errCh:
		return nil, fmt.Errorf("authentication failed: %w", err)
	case <-ctx.Done():
		return nil, fmt.Errorf("authentication timeout after 5 minutes")
	}

	// Create token object
	token := &oauth2.Token{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		// AniList tokens are valid for 1 year
		Expiry: time.Now().Add(365 * 24 * time.Hour),
	}

	// Save token if callback is provided
	if saveToken != nil {
		if err := saveToken(token); err != nil {
			return nil, fmt.Errorf("failed to save token: %w", err)
		}
	}

	fmt.Println("Authentication successful!")
	return token, nil
}

// ExtractTokenFromInput extracts the access token from various input formats
// Handles: raw token, URL with token, or token with extra parameters
func ExtractTokenFromInput(input string) string {
	input = strings.TrimSpace(input)

	// Check if it's a URL containing the token
	if strings.Contains(input, "access_token=") {
		// Extract token from URL fragment
		parts := strings.Split(input, "access_token=")
		if len(parts) > 1 {
			token := parts[1]
			// Remove any additional parameters (like &token_type=Bearer)
			if idx := strings.Index(token, "&"); idx != -1 {
				token = token[:idx]
			}
			// Remove any trailing fragments
			if idx := strings.Index(token, "#"); idx != -1 {
				token = token[:idx]
			}
			return strings.TrimSpace(token)
		}
	}

	// Check if the input itself has parameters appended (without the URL)
	if strings.Contains(input, "&") {
		parts := strings.Split(input, "&")
		return strings.TrimSpace(parts[0])
	}

	// It's already just the token, return it
	return input
}

// exchangeCodeForToken exchanges an authorization code for an access token
func exchangeCodeForToken(code string, authConfig BrowserAuthConfig) (string, error) {
	tokenURL := fmt.Sprintf("%s/token", browserAuthOAuthURL)
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {authConfig.ClientID},
		"client_secret": {authConfig.ClientSecret},
		"redirect_uri":  {authConfig.RedirectURI},
		"code":          {code},
	}

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed with status: %d", resp.StatusCode)
	}

	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResponse.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}

	return tokenResponse.AccessToken, nil
}
