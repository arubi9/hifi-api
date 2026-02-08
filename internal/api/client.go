package api

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"time"

	json "github.com/goccy/go-json"
	"github.com/mrpir/hifi-tui/internal/logger"
)

const (
	defaultBaseURL = "https://triton.squid.wtf"
	maxRetries     = 3
)

// Client is the API client for the Tidal proxy.
type Client struct {
	base    string
	apiKey  string
	quality string
	http    *http.Client
	cdn     *http.Client // separate client for CDN downloads (HTTP/1.1 friendly)
}

// NewClient creates an API client with tuned transports.
func NewClient(quality string) *Client {
	base := os.Getenv("API_BASE")
	if base == "" {
		base = defaultBaseURL
	}
	apiKey := os.Getenv("API_KEY")

	// API transport: HTTP/2 multiplexed for search/batch
	apiTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		MaxConnsPerHost:     50,
		DisableCompression:  true,
		ReadBufferSize:      256 * 1024,
		WriteBufferSize:     256 * 1024,
		ForceAttemptHTTP2:   true,
		IdleConnTimeout:     30 * time.Second,
	}

	// CDN transport: large file downloads
	cdnTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     20,
		DisableCompression:  true,
		ReadBufferSize:      256 * 1024,
		WriteBufferSize:     256 * 1024,
		IdleConnTimeout:     30 * time.Second,
	}

	return &Client{
		base:    base,
		apiKey:  apiKey,
		quality: quality,
		http: &http.Client{
			Transport: apiTransport,
			Timeout:   60 * time.Second,
		},
		cdn: &http.Client{
			Transport: cdnTransport,
			Timeout:   120 * time.Second,
		},
	}
}

// SetQuality updates the default quality.
func (c *Client) SetQuality(q string) {
	c.quality = q
}

// Quality returns the current quality setting.
func (c *Client) Quality() string {
	return c.quality
}

// CDNClient returns the CDN HTTP client for direct downloads.
func (c *Client) CDNClient() *http.Client {
	return c.cdn
}

// doGet performs a GET request with retry and backoff.
func (c *Client) doGet(ctx context.Context, path string) ([]byte, error) {
	url := c.base + path
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			logger.Log.Debug("retrying request", "url", url, "attempt", attempt, "backoff", backoff)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		if c.apiKey != "" {
			req.Header.Set("X-API-Key", c.apiKey)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}
		return body, nil
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doPost performs a POST request with retry and backoff.
func (c *Client) doPost(ctx context.Context, path string, payload interface{}) ([]byte, error) {
	url := c.base + path
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, io.NopCloser(
			&bytesReader{data: body},
		))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			req.Header.Set("X-API-Key", c.apiKey)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
		}
		return respBody, nil
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// bytesReader wraps a []byte for io.Reader without importing bytes.
type bytesReader struct {
	data []byte
	off  int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}
