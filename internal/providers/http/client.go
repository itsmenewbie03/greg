package http

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-resty/resty/v2"
)

// Client wraps resty.Client with retry logic and timeout handling
type Client struct {
	resty      *resty.Client
	maxRetries int
	timeout    time.Duration
	debug      bool
	logger     *slog.Logger
}

// ClientConfig holds configuration for the HTTP client
type ClientConfig struct {
	Timeout    time.Duration
	MaxRetries int
	UserAgent  string
	Debug      bool
	Logger     *slog.Logger
}

// DefaultClientConfig returns sensible defaults for HTTP client
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		UserAgent:  "greg/1.0",
	}
}

// NewClient creates a new HTTP client with the given configuration
func NewClient(config ClientConfig) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.UserAgent == "" {
		config.UserAgent = "greg/1.0"
	}

	restyClient := resty.New().
		SetTimeout(config.Timeout).
		SetRetryCount(config.MaxRetries).
		SetRetryWaitTime(1*time.Second).
		SetRetryMaxWaitTime(5*time.Second).
		SetHeader("User-Agent", config.UserAgent).
		SetHeader("Accept", "application/json, text/html, */*").
		SetHeader("Accept-Language", "en-US,en;q=0.9")

	// Add retry conditions
	restyClient.AddRetryCondition(func(r *resty.Response, err error) bool {
		// Retry on network errors
		if err != nil {
			return true
		}
		// Retry on 5xx server errors and 429 rate limiting
		return r.StatusCode() >= 500 || r.StatusCode() == 429
	})

	client := &Client{
		resty:      restyClient,
		maxRetries: config.MaxRetries,
		timeout:    config.Timeout,
		debug:      config.Debug,
		logger:     config.Logger,
	}

	// Enable debug logging if requested
	if config.Debug && config.Logger != nil {
		restyClient.OnBeforeRequest(func(c *resty.Client, r *resty.Request) error {
			client.logRequest(r)
			return nil
		})
		restyClient.OnAfterResponse(func(c *resty.Client, r *resty.Response) error {
			client.logResponse(r)
			return nil
		})
	}

	return client
}

// Get performs a GET request with context support
func (c *Client) Get(ctx context.Context, url string, headers map[string]string) (*resty.Response, error) {
	req := c.resty.R().SetContext(ctx)

	// Set custom headers
	for key, value := range headers {
		req.SetHeader(key, value)
	}

	resp, err := req.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET request failed for %s: %w", url, err)
	}

	// Check for HTTP errors
	if resp.StatusCode() >= 400 {
		return resp, fmt.Errorf("HTTP error %d for %s: %s", resp.StatusCode(), url, resp.String())
	}

	return resp, nil
}

// Post performs a POST request with context support
func (c *Client) Post(ctx context.Context, url string, body interface{}, headers map[string]string) (*resty.Response, error) {
	req := c.resty.R().
		SetContext(ctx).
		SetBody(body)

	// Set custom headers
	for key, value := range headers {
		req.SetHeader(key, value)
	}

	resp, err := req.Post(url)
	if err != nil {
		return nil, fmt.Errorf("POST request failed for %s: %w", url, err)
	}

	// Check for HTTP errors
	if resp.StatusCode() >= 400 {
		return resp, fmt.Errorf("HTTP error %d for %s: %s", resp.StatusCode(), url, resp.String())
	}

	return resp, nil
}

// SetHeader sets a default header for all requests
func (c *Client) SetHeader(key, value string) {
	c.resty.SetHeader(key, value)
}

// SetHeaders sets multiple default headers
func (c *Client) SetHeaders(headers map[string]string) {
	c.resty.SetHeaders(headers)
}

// GetTimeout returns the configured timeout
func (c *Client) GetTimeout() time.Duration {
	return c.timeout
}

// GetMaxRetries returns the configured max retries
func (c *Client) GetMaxRetries() int {
	return c.maxRetries
}

// GetRestyClient returns the underlying resty client
// This is useful for integrations that need the raw client (e.g., GraphQL)
func (c *Client) GetRestyClient() *resty.Client {
	return c.resty
}

// logRequest logs HTTP request details
func (c *Client) logRequest(r *resty.Request) {
	if c.logger == nil {
		return
	}

	c.logger.Debug("HTTP Request",
		"method", r.Method,
		"url", r.URL,
		"headers", r.Header,
	)

	if r.Body != nil {
		c.logger.Debug("Request Body",
			"body", fmt.Sprintf("%v", r.Body),
		)
	}
}

// logResponse logs HTTP response details
func (c *Client) logResponse(r *resty.Response) {
	if c.logger == nil {
		return
	}

	c.logger.Debug("HTTP Response",
		"status", r.StatusCode(),
		"status_text", r.Status(),
		"url", r.Request.URL,
		"headers", r.Header(),
		"time", r.Time(),
	)

	bodyStr := r.String()
	if len(bodyStr) > 1000 {
		bodyStr = bodyStr[:1000] + "... (truncated)"
	}
	c.logger.Debug("Response Body",
		"body", bodyStr,
	)
}
