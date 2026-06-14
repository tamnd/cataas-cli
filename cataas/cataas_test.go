package cataas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	c := NewClient()
	c.BaseURL = srv.URL
	c.Rate = 0
	c.Retries = 1
	return c
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestRandomCat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/cat") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("json") != "true" {
			t.Error("missing json=true query param")
		}
		resp := apiCat{
			RawID: "abc123",
			URL:   "https://cataas.com/cat/abc123",
			Tags:  []string{"cute"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	cat, err := c.RandomCat(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if cat.ID != "abc123" {
		t.Errorf("cat.ID = %q, want abc123", cat.ID)
	}
	if cat.URL != "https://cataas.com/cat/abc123" {
		t.Errorf("cat.URL = %q", cat.URL)
	}
	if len(cat.Tags) != 1 || cat.Tags[0] != "cute" {
		t.Errorf("cat.Tags = %v, want [cute]", cat.Tags)
	}
}

func TestRandomCatWithTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cat/cute" {
			t.Errorf("unexpected path %q, want /cat/cute", r.URL.Path)
		}
		resp := apiCat{
			RawID: "def456",
			URL:   "https://cataas.com/cat/def456",
			Tags:  []string{"cute"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	cat, err := c.RandomCat(context.Background(), "cute")
	if err != nil {
		t.Fatal(err)
	}
	if cat.ID != "def456" {
		t.Errorf("cat.ID = %q, want def456", cat.ID)
	}
}

func TestListCats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cats" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("unexpected limit %q", r.URL.Query().Get("limit"))
		}
		raws := []apiCat{
			{RawID: "cat1", URL: "https://cataas.com/cat/cat1", Tags: []string{"cute"}},
			{RawID: "cat2", URL: "https://cataas.com/cat/cat2", Tags: []string{"funny"}},
			{RawID: "", URL: "https://cataas.com/cat/", Tags: nil}, // empty id, should be filtered
		}
		_ = json.NewEncoder(w).Encode(raws)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	cats, err := c.ListCats(context.Background(), "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 2 {
		t.Errorf("len(cats) = %d, want 2 (empty id filtered)", len(cats))
	}
	if cats[0].ID != "cat1" {
		t.Errorf("cats[0].ID = %q, want cat1", cats[0].ID)
	}
}

func TestListCatsWithTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tags") != "cute" {
			t.Errorf("unexpected tags param %q", r.URL.Query().Get("tags"))
		}
		raws := []apiCat{
			{RawID: "cat1", URL: "https://cataas.com/cat/cat1", Tags: []string{"cute"}},
		}
		_ = json.NewEncoder(w).Encode(raws)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	cats, err := c.ListCats(context.Background(), "cute", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 1 {
		t.Errorf("len(cats) = %d, want 1", len(cats))
	}
}

func TestListTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		tags := []string{"", "a", "cute", "funny", "#christmascat", "adorable"}
		_ = json.NewEncoder(w).Encode(tags)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	tags, err := c.ListTags(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	// "" and "a" should be filtered (len < 2)
	if len(tags) != 4 {
		t.Errorf("len(tags) = %d, want 4 (short tags filtered)", len(tags))
	}
	if tags[0].Name != "cute" {
		t.Errorf("tags[0].Name = %q, want cute", tags[0].Name)
	}
}

func TestListTagsWithLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tags := []string{"cute", "funny", "adorable", "angry", "sleepy"}
		_ = json.NewEncoder(w).Encode(tags)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	tags, err := c.ListTags(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Errorf("len(tags) = %d, want 3", len(tags))
	}
}
