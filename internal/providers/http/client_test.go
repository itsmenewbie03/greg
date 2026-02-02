package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Run("creates client with default config", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		assert.NotNil(t, client)
		assert.Equal(t, 30*time.Second, client.GetTimeout())
		assert.Equal(t, 3, client.GetMaxRetries())
	})

	t.Run("creates client with custom config", func(t *testing.T) {
		config := ClientConfig{
			Timeout:    10 * time.Second,
			MaxRetries: 5,
			UserAgent:  "test-agent/1.0",
		}
		client := NewClient(config)

		assert.NotNil(t, client)
		assert.Equal(t, 10*time.Second, client.GetTimeout())
		assert.Equal(t, 5, client.GetMaxRetries())
	})

	t.Run("uses defaults for zero values", func(t *testing.T) {
		config := ClientConfig{}
		client := NewClient(config)

		assert.Equal(t, 30*time.Second, client.GetTimeout())
		assert.Equal(t, 3, client.GetMaxRetries())
	})
}

func TestClient_Get(t *testing.T) {
	t.Run("successful GET request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "/test", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))
		}))
		defer server.Close()

		client := NewClient(DefaultClientConfig())
		resp, err := client.Get(context.Background(), server.URL+"/test", nil)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
		assert.Contains(t, string(resp.Body()), "ok")
	})

	t.Run("GET request with custom headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "custom-value", r.Header.Get("X-Custom-Header"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewClient(DefaultClientConfig())
		headers := map[string]string{
			"X-Custom-Header": "custom-value",
		}
		resp, err := client.Get(context.Background(), server.URL, headers)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})

	t.Run("handles 404 error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
		}))
		defer server.Close()

		client := NewClient(DefaultClientConfig())
		_, err := client.Get(context.Background(), server.URL, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewClient(ClientConfig{
			Timeout:    10 * time.Second,
			MaxRetries: 0, // Don't retry for this test
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := client.Get(ctx, server.URL, nil)

		require.Error(t, err)
	})

	t.Run("handles server errors with retry", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts < 3 {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer server.Close()

		client := NewClient(ClientConfig{
			Timeout:    10 * time.Second,
			MaxRetries: 3,
		})

		resp, err := client.Get(context.Background(), server.URL, nil)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
		assert.Equal(t, 3, attempts) // Should have retried twice and succeeded on third
	})
}

func TestClient_Post(t *testing.T) {
	t.Run("successful POST request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"created": true}`))
		}))
		defer server.Close()

		client := NewClient(DefaultClientConfig())
		body := map[string]string{"key": "value"}
		resp, err := client.Post(context.Background(), server.URL, body, nil)

		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode())
	})

	t.Run("POST request with custom headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer token123", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewClient(DefaultClientConfig())
		headers := map[string]string{
			"Authorization": "Bearer token123",
		}
		resp, err := client.Post(context.Background(), server.URL, nil, headers)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})

	t.Run("handles POST errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("bad request"))
		}))
		defer server.Close()

		client := NewClient(DefaultClientConfig())
		_, err := client.Post(context.Background(), server.URL, nil, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "400")
	})
}

func TestClient_SetHeaders(t *testing.T) {
	t.Run("sets default headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "test-value", r.Header.Get("X-Test-Header"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewClient(DefaultClientConfig())
		client.SetHeader("X-Test-Header", "test-value")

		resp, err := client.Get(context.Background(), server.URL, nil)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})

	t.Run("sets multiple default headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "value1", r.Header.Get("X-Header-1"))
			assert.Equal(t, "value2", r.Header.Get("X-Header-2"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewClient(DefaultClientConfig())
		client.SetHeaders(map[string]string{
			"X-Header-1": "value1",
			"X-Header-2": "value2",
		})

		resp, err := client.Get(context.Background(), server.URL, nil)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})
}
