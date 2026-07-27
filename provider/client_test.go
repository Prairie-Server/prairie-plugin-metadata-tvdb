package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func tvdbLoginOK(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "t"}})
}

func TestNewClientDefaultRate(t *testing.T) {
	c := NewClient(0)
	if c.limiter == nil {
		t.Fatal("limiter nil")
	}
}

func TestClientMovieSeasonTranslationEndpoints(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			tvdbLoginOK(w)
		case r.URL.Path == "/movies/9/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"id": 9, "name": "Movie"}})
		case r.URL.Path == "/seasons/3/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"id": 3, "number": 1}})
		case r.URL.Path == "/movies/9/translations/eng":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"name": "EN", "overview": "ov", "language": "eng"}})
		case r.URL.Path == "/seasons/3/translations/eng":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"name": "Season", "overview": "sov", "language": "eng"}})
		case r.URL.Path == "/series/1/translations/eng":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"name": "S", "language": "eng"}})
		case r.URL.Path == "/episodes/5/translations/eng":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"name": "E", "language": "eng"}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	c := NewClient(1000)
	c.SetBaseURL(server.URL)
	ctx := context.Background()
	if m, err := c.GetMovieExtended(ctx, 9); err != nil || m.ID != 9 {
		t.Fatalf("movie=%v err=%v", m, err)
	}
	if s, err := c.GetSeasonExtended(ctx, 3); err != nil || s.ID != 3 {
		t.Fatalf("season=%v err=%v", s, err)
	}
	if tr, err := c.GetMovieTranslation(ctx, 9, "eng"); err != nil || tr.Name != "EN" {
		t.Fatalf("movie tr=%v err=%v", tr, err)
	}
	if tr, err := c.GetSeasonTranslation(ctx, 3, "eng"); err != nil || tr.Name != "Season" {
		t.Fatalf("season tr=%v err=%v", tr, err)
	}
	if tr, err := c.GetSeriesTranslation(ctx, 1, "eng"); err != nil || tr.Name != "S" {
		t.Fatalf("series tr=%v err=%v", tr, err)
	}
	if tr, err := c.GetEpisodeTranslation(ctx, 5, "eng"); err != nil || tr.Name != "E" {
		t.Fatalf("ep tr=%v err=%v", tr, err)
	}
}

func TestClientDoGetErrorPaths(t *testing.T) {
	t.Parallel()
	t.Run("4xx", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login" {
				tvdbLoginOK(w)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "missing"})
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		var dest map[string]any
		if err := c.doGet(context.Background(), "/missing", &dest); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("429 then ok", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login" {
				tvdbLoginOK(w)
				return
			}
			if calls.Add(1) == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{}})
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		var dest apiResponse[map[string]any]
		if err := c.doGet(context.Background(), "/x", &dest); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("429 canceled", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login" {
				tvdbLoginOK(w)
				return
			}
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		var dest map[string]any
		if err := c.doGet(ctx, "/x", &dest); err == nil {
			t.Fatal("expected cancel")
		}
	})
	t.Run("5xx canceled", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login" {
				tvdbLoginOK(w)
				return
			}
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		var dest map[string]any
		if err := c.doGet(ctx, "/x", &dest); err == nil {
			t.Fatal("expected cancel")
		}
	})
	t.Run("invalid retry-after then ok", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login" {
				tvdbLoginOK(w)
				return
			}
			if calls.Add(1) == 1 {
				w.Header().Set("Retry-After", "nope")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		var dest map[string]any
		if err := c.doGet(context.Background(), "/x", &dest); err != nil {
			t.Fatal(err)
		}
	})
}

func TestEpisodesCachePurge(t *testing.T) {
	t.Parallel()
	c := NewClient(1000)
	c.episodesCache = map[string]episodesCacheEntry{}
	for i := 0; i < episodesCacheMaxEntries; i++ {
		c.episodesCache[string(rune(i))] = episodesCacheEntry{
			data: &seriesEpisodes{}, fetchedAt: time.Now().Add(-episodesCacheTTL - time.Second),
		}
	}
	c.storeEpisodesCache("fresh", &seriesEpisodes{})
	if _, ok := c.lookupEpisodesCache("fresh"); !ok {
		t.Fatal("fresh missing")
	}
	if _, ok := c.lookupEpisodesCache("missing"); ok {
		t.Fatal("unexpected hit")
	}
}

func TestClientAuthenticateFailuresAnd401Refresh(t *testing.T) {
	t.Parallel()

	t.Run("login http error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("nope"))
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		if err := c.authenticate(context.Background()); err == nil {
			t.Fatal("expected login error")
		}
	})

	t.Run("empty token", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": ""}})
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		if err := c.authenticate(context.Background()); err == nil {
			t.Fatal("expected empty token")
		}
	})

	t.Run("bad login json", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		if err := c.authenticate(context.Background()); err == nil {
			t.Fatal("expected decode error")
		}
	})

	t.Run("401 refresh then success", func(t *testing.T) {
		t.Parallel()
		var gets atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/login" {
				tvdbLoginOK(w)
				return
			}
			n := gets.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"ok": true}})
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		var dest apiResponse[map[string]any]
		if err := c.doGet(context.Background(), "/x", &dest); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("401 refresh fails twice", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login" {
				tvdbLoginOK(w)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		var dest map[string]any
		if err := c.doGet(context.Background(), "/x", &dest); err == nil {
			t.Fatal("expected auth failure")
		}
	})

	t.Run("4xx without message and decode error and 5xx success", func(t *testing.T) {
		t.Parallel()
		var mode atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login" {
				tvdbLoginOK(w)
				return
			}
			switch mode.Load() {
			case 0:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("plain"))
			case 1:
				_, _ = w.Write([]byte("{"))
			case 2:
				w.WriteHeader(http.StatusBadGateway)
			}
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		var dest map[string]any
		mode.Store(0)
		if err := c.doGet(context.Background(), "/a", &dest); err == nil {
			t.Fatal("expected 4xx")
		}
		mode.Store(1)
		if err := c.doGet(context.Background(), "/b", &dest); err == nil {
			t.Fatal("expected decode")
		}
	})

	t.Run("5xx then success", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login" {
				tvdbLoginOK(w)
				return
			}
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}))
		t.Cleanup(server.Close)
		c := NewClient(1000)
		c.SetBaseURL(server.URL)
		var dest map[string]any
		if err := c.doGet(context.Background(), "/c", &dest); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("canceled limiter", func(t *testing.T) {
		t.Parallel()
		c := NewClient(1000)
		c.SetBaseURL("http://127.0.0.1:1")
		c.token = "x"
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var dest map[string]any
		if err := c.doGet(ctx, "/x", &dest); err == nil {
			t.Fatal("expected cancel")
		}
	})

	t.Run("refresh already updated", func(t *testing.T) {
		t.Parallel()
		c := NewClient(1000)
		c.token = "new"
		if err := c.refreshToken(context.Background(), "old"); err != nil {
			t.Fatal(err)
		}
	})
}
