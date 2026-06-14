// Package cataas is the library behind the cataas command line:
// the HTTP client, request shaping, and the typed data models for cataas.com.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package cataas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Host is the site this client talks to.
const Host = "cataas.com"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// DefaultUserAgent identifies the client to cataas honestly.
const DefaultUserAgent = "cataas-cli/0.1 (tamnd87@gmail.com)"

// Cat is a single cat record from the CATAAS API.
type Cat struct {
	ID   string   `json:"id"   kit:"id"`
	URL  string   `json:"url"`
	Tags []string `json:"tags"`
}

// Tag is a tag record from the CATAAS API.
type Tag struct {
	Name string `json:"name" kit:"id"`
}

// apiCat is the raw wire shape from the API before we remap _id → id.
type apiCat struct {
	RawID string   `json:"_id"`
	URL   string   `json:"url"`
	Tags  []string `json:"tags"`
}

func (a apiCat) toCat() *Cat {
	return &Cat{
		ID:   a.RawID,
		URL:  a.URL,
		Tags: a.Tags,
	}
}

// Config holds the client settings.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns sensible defaults for the client.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		Rate:      300 * time.Millisecond,
		Timeout:   15 * time.Second,
		Retries:   3,
		UserAgent: DefaultUserAgent,
	}
}

// Client talks to cataas.com over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// RandomCat returns a random cat, optionally filtered by tag.
// If tag is non-empty, uses GET /cat/<tag>?json=true, otherwise GET /cat?json=true.
func (c *Client) RandomCat(ctx context.Context, tag string) (*Cat, error) {
	var endpoint string
	if tag != "" {
		endpoint = c.BaseURL + "/cat/" + url.PathEscape(tag) + "?json=true"
	} else {
		endpoint = c.BaseURL + "/cat?json=true"
	}
	body, err := c.Get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var raw apiCat
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse cat: %w", err)
	}
	cat := raw.toCat()
	if cat.ID == "" {
		return nil, fmt.Errorf("random cat returned empty id")
	}
	return cat, nil
}

// ListCats returns a list of cats, optionally filtered by tag, up to limit.
func (c *Client) ListCats(ctx context.Context, tag string, limit int) ([]*Cat, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if tag != "" {
		q.Set("tags", tag)
	}
	endpoint := c.BaseURL + "/api/cats?" + q.Encode()
	body, err := c.Get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var raws []apiCat
	if err := json.Unmarshal(body, &raws); err != nil {
		return nil, fmt.Errorf("parse cats: %w", err)
	}
	var out []*Cat
	for _, raw := range raws {
		if raw.RawID == "" {
			continue
		}
		out = append(out, raw.toCat())
	}
	return out, nil
}

// ListTags returns all available tags, filtering out tags shorter than 2 characters.
// If limit > 0, returns at most limit tags.
func (c *Client) ListTags(ctx context.Context, limit int) ([]*Tag, error) {
	body, err := c.Get(ctx, c.BaseURL+"/api/tags")
	if err != nil {
		return nil, err
	}
	var raw []string
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse tags: %w", err)
	}
	var out []*Tag
	for _, t := range raw {
		if len(t) < 2 {
			continue
		}
		out = append(out, &Tag{Name: t})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
