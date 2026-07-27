package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prairie-server/prairie-plugin-metadata-tvdb/metadata"
)

func TestProviderSearchByTitleIncludesRemoteIDs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"token": "test-token",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/search":
			if r.URL.Query().Get("query") != "10 Tokyo Warriors" {
				t.Fatalf("query = %q, want 10 Tokyo Warriors", r.URL.Query().Get("query"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": []map[string]any{{
					"name":             "倒凶十将伝",
					"aliases":          []string{"10 Tokyo Warriors"},
					"primary_language": "jpn",
					"year":             "1999",
					"tvdb_id":          "420105",
					"overview":         "Ten warriors defend Tokyo.",
					"remote_ids": []map[string]any{
						{"type": 12, "id": "201992", "sourceName": "TheMovieDB.com"},
						{"type": 2, "id": "tt18076310", "sourceName": "IMDB"},
					},
				}},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	results, err := p.Search(context.Background(), metadata.SearchQuery{
		Title:       "10 Tokyo Warriors",
		ContentType: "series",
		Language:    "en",
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	ids := results[0].ProviderIDs
	if ids["tvdb"] != "420105" || ids["tmdb"] != "201992" || ids["imdb"] != "tt18076310" {
		t.Fatalf("provider ids = %+v, want tvdb/tmdb/imdb", ids)
	}
	if results[0].Name != "倒凶十将伝" || results[0].OriginalLanguage != "ja" || !results[0].TitleIsFallback {
		t.Fatalf("title metadata = (%q, %q, %v)", results[0].Name, results[0].OriginalLanguage, results[0].TitleIsFallback)
	}
	if len(results[0].TitleAliases) != 1 || results[0].TitleAliases[0].Title != "10 Tokyo Warriors" {
		t.Fatalf("aliases = %#v, want English search alias", results[0].TitleAliases)
	}
}

func TestGetSeriesMetadataIncludesSourceNameRemoteIDs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"token": "test-token"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/series/100/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":       100,
					"name":     "Series",
					"overview": "Series overview",
					"remoteIds": []map[string]any{
						{"type": 0, "id": "201992", "sourceName": "TheMovieDB.com"},
						{"type": 0, "id": "tt18076310", "sourceName": "IMDb"},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "100"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	if result.ProviderIDs["tvdb"] != "100" || result.ProviderIDs["tmdb"] != "201992" || result.ProviderIDs["imdb"] != "tt18076310" {
		t.Fatalf("provider ids = %+v, want tvdb/tmdb/imdb", result.ProviderIDs)
	}
}

func TestFillRemoteIDsUsesTypeAndSourceNameWithoutOverwrite(t *testing.T) {
	t.Parallel()

	ids := map[string]string{
		"imdb": "nm-existing",
		"tmdb": "existing-tmdb",
	}
	fillRemoteIDs(ids, []RemoteID{
		{Type: 0, ID: "tt-source", SourceName: "IMDb"},
		{Type: 0, ID: "source-tmdb", SourceName: "The Movie Database"},
		{Type: 2, ID: "tt-type", SourceName: ""},
		{Type: 12, ID: "type-tmdb", SourceName: ""},
	})

	if ids["imdb"] != "nm-existing" {
		t.Fatalf("imdb overwritten: got %q", ids["imdb"])
	}
	if ids["tmdb"] != "existing-tmdb" {
		t.Fatalf("tmdb overwritten: got %q", ids["tmdb"])
	}

	ids = map[string]string{}
	fillRemoteIDs(ids, []RemoteID{
		{Type: 0, ID: "30773-the-yogi-bear-show", SourceName: "TheMovieDB.com"},
		{Type: 0, ID: "not-an-imdb-id", SourceName: "imdb.com"},
		{Type: 0, ID: "tt123", SourceName: "imdb.com"},
		{Type: 0, ID: "nm1234567", SourceName: "imdb.com"},
		{Type: 0, ID: "201992", SourceName: "TheMovieDB.com"},
		{Type: 0, ID: "TT18076310", SourceName: "imdb.com"},
	})

	if ids["tmdb"] != "201992" || ids["imdb"] != "tt18076310" {
		t.Fatalf("provider ids = %+v, want source-name tmdb/imdb", ids)
	}
}

func TestGetImagesReturnsArtworkImageURLs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"token": "test-token",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/series/99/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":   99,
					"name": "Series",
					"artworks": []map[string]any{
						{
							"id":        1,
							"type":      2,
							"image":     "https://artworks.example/poster-original.jpg",
							"thumbnail": "https://artworks.example/poster-thumb.jpg",
							"width":     2000,
							"height":    3000,
							"score":     10,
						},
						{
							"id":        2,
							"type":      3,
							"image":     "https://artworks.example/background-original.jpg",
							"thumbnail": "",
							"width":     3840,
							"height":    2160,
							"score":     8,
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ProviderIDs: map[string]string{"tvdb": "99"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("GetImages() error = %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("len(images) = %d, want 2", len(images))
	}

	got := map[metadata.ImageType]string{}
	for _, img := range images {
		got[img.Type] = img.URL
	}

	if got[metadata.ImagePoster] != "https://artworks.example/poster-original.jpg" {
		t.Fatalf("poster URL = %q", got[metadata.ImagePoster])
	}
	if got[metadata.ImageBackdrop] != "https://artworks.example/background-original.jpg" {
		t.Fatalf("backdrop URL = %q", got[metadata.ImageBackdrop])
	}
}

func TestGetImagesPrefersTVDBPrimaryPoster(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"token": "test-token",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/series/99/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":    99,
					"name":  "Series",
					"image": "https://artworks.example/poster-primary.jpg",
					"artworks": []map[string]any{
						{
							"id":       1,
							"type":     2,
							"image":    "https://artworks.example/poster-primary.jpg",
							"language": "eng",
							"width":    2000,
							"height":   3000,
							"score":    10,
						},
						{
							"id":       2,
							"type":     2,
							"image":    "https://artworks.example/poster-textless.jpg",
							"language": "",
							"width":    2000,
							"height":   3000,
							"score":    11,
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ProviderIDs: map[string]string{"tvdb": "99"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("GetImages() error = %v", err)
	}

	var primary, textless *metadata.RemoteImage
	for i := range images {
		switch images[i].URL {
		case "https://artworks.example/poster-primary.jpg":
			primary = &images[i]
		case "https://artworks.example/poster-textless.jpg":
			textless = &images[i]
		}
	}

	if primary == nil {
		t.Fatal("primary poster missing from GetImages() result")
	}
	if textless == nil {
		t.Fatal("alternate poster missing from GetImages() result")
	}
	if primary.Language != "en" {
		t.Fatalf("primary language = %q, want en", primary.Language)
	}
	if primary.Rating <= textless.Rating {
		t.Fatalf("primary rating = %v, textless rating = %v; want primary > textless", primary.Rating, textless.Rating)
	}
}

func TestGetImagesAddsPrimaryPosterWhenArtworkListMissesIt(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"token": "test-token",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/series/99/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":    99,
					"name":  "Series",
					"image": "https://artworks.example/poster-primary.jpg",
					"artworks": []map[string]any{
						{
							"id":       2,
							"type":     2,
							"image":    "https://artworks.example/poster-alt.jpg",
							"language": "",
							"width":    2000,
							"height":   3000,
							"score":    11,
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	images, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ProviderIDs: map[string]string{"tvdb": "99"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("GetImages() error = %v", err)
	}

	var primary, alt *metadata.RemoteImage
	for i := range images {
		switch images[i].URL {
		case "https://artworks.example/poster-primary.jpg":
			primary = &images[i]
		case "https://artworks.example/poster-alt.jpg":
			alt = &images[i]
		}
	}

	if primary == nil {
		t.Fatal("primary poster was not appended to GetImages() result")
	}
	if alt == nil {
		t.Fatal("alternate poster missing from GetImages() result")
	}
	if primary.Rating <= alt.Rating {
		t.Fatalf("primary rating = %v, alt rating = %v; want primary > alt", primary.Rating, alt.Rating)
	}
}

// ---------------------------------------------------------------------------
// Translation tests
// ---------------------------------------------------------------------------

// newTranslationTestServer creates a test server that serves a Japanese series
// with embedded translations and per-entity translation endpoints.
func newTranslationTestServer(t *testing.T, translationCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"token": "test-token"},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/series/100/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":               100,
					"name":             "Original Japanese Title",
					"overview":         "Original Japanese overview",
					"originalLanguage": "jpn",
					"image":            "https://example.com/poster.jpg",
					"translations": map[string]any{
						"nameTranslations": []map[string]any{
							{"language": "jpn", "name": "Original Japanese Title"},
							{"language": "eng", "name": "English Series Title"},
						},
						"overviewTranslations": []map[string]any{
							{"language": "jpn", "overview": "Original Japanese overview"},
							{"language": "eng", "overview": "English series overview"},
						},
					},
					"seasons": []map[string]any{
						{"id": 200, "seriesId": 100, "number": 1, "type": map[string]any{"id": 1, "name": "Aired Order"}},
					},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/series/100/translations/eng":
			if translationCalls != nil {
				translationCalls.Add(1)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"name":     "English Series Title",
					"overview": "English series overview",
					"language": "eng",
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/series/100/episodes/official":
			// Bulk base (original-language) episode list for the whole series.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"series": map[string]any{"id": 100, "originalLanguage": "jpn"},
					"episodes": []map[string]any{
						{"id": 301, "name": "Japanese Ep 1", "overview": "JP overview 1", "number": 1, "seasonNumber": 1},
						{"id": 302, "name": "Japanese Ep 2", "overview": "JP overview 2", "number": 2, "seasonNumber": 1},
						{"id": 303, "name": "Japanese Ep 3", "overview": "JP overview 3", "number": 3, "seasonNumber": 1},
					},
				},
				"links": map[string]any{"next": nil},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/series/100/episodes/official/eng":
			// Bulk translated episode list — one call for the whole series.
			if translationCalls != nil {
				translationCalls.Add(1)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"series": map[string]any{"id": 100, "originalLanguage": "jpn"},
					"episodes": []map[string]any{
						{"id": 301, "name": "English Ep 1", "overview": "EN overview 1", "number": 1, "seasonNumber": 1},
						{"id": 302, "name": "English Ep 2", "overview": "EN overview 2", "number": 2, "seasonNumber": 1},
						{"id": 303, "name": "English Ep 3", "overview": "EN overview 3", "number": 3, "seasonNumber": 1},
					},
				},
				"links": map[string]any{"next": nil},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/seasons/200/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":       200,
					"seriesId": 100,
					"number":   1,
					"type":     map[string]any{"id": 1, "name": "Aired Order"},
					"episodes": []map[string]any{
						{"id": 301, "name": "Japanese Ep 1", "overview": "JP overview 1", "number": 1, "seasonNumber": 1},
						{"id": 302, "name": "Japanese Ep 2", "overview": "JP overview 2", "number": 2, "seasonNumber": 1},
						{"id": 303, "name": "Japanese Ep 3", "overview": "JP overview 3", "number": 3, "seasonNumber": 1},
					},
				},
			})

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/episodes/") && strings.HasSuffix(r.URL.Path, "/translations/eng"):
			if translationCalls != nil {
				translationCalls.Add(1)
			}
			// Extract episode ID from path.
			parts := strings.Split(r.URL.Path, "/")
			epID := parts[2]
			names := map[string]string{"301": "English Ep 1", "302": "English Ep 2", "303": "English Ep 3"}
			overviews := map[string]string{"301": "EN overview 1", "302": "EN overview 2", "303": "EN overview 3"}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"name":     names[epID],
					"overview": overviews[epID],
					"language": "eng",
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/seasons/200/translations/eng":
			if translationCalls != nil {
				translationCalls.Add(1)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"name":     "Season 1",
					"overview": "English season overview",
					"language": "eng",
				},
			})

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
}

func TestGetSeriesMetadata_TranslatesNonNativeLanguage(t *testing.T) {
	t.Parallel()

	server := newTranslationTestServer(t, nil)
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "100"},
		ContentType: "series",
		Language:    "en",
	})
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	if result.Title != "English Series Title" {
		t.Fatalf("Title = %q, want %q", result.Title, "English Series Title")
	}
	if result.Overview != "English series overview" {
		t.Fatalf("Overview = %q, want %q", result.Overview, "English series overview")
	}
	if result.TitleLanguage != "en" || result.TitleIsFallback || result.OriginalLanguage != "ja" || result.OriginalTitle != "Original Japanese Title" {
		t.Fatalf("title language metadata = title:%q fallback:%v original_language:%q original_title:%q", result.TitleLanguage, result.TitleIsFallback, result.OriginalLanguage, result.OriginalTitle)
	}
	if !result.TitleAliasesComplete {
		t.Fatal("full TVDB extended response must mark title aliases complete")
	}
	foundOriginal, foundEnglish := false, false
	for _, alias := range result.TitleAliases {
		foundOriginal = foundOriginal || alias.Title == "Original Japanese Title" && alias.Language == "ja" && alias.Kind == "original"
		foundEnglish = foundEnglish || alias.Title == "English Series Title" && alias.Language == "en" && alias.Kind == "localized"
	}
	if !foundOriginal || !foundEnglish {
		t.Fatalf("title aliases = %#v, want Japanese original and English localized aliases", result.TitleAliases)
	}
}

func TestGetSeriesMetadata_SkipsTranslationWhenLanguageMatchesOriginal(t *testing.T) {
	t.Parallel()

	var translationCalls atomic.Int32
	server := newTranslationTestServer(t, &translationCalls)
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "100"},
		ContentType: "series",
		Language:    "ja", // matches originalLanguage "jpn"
	})
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	// Should use original data without fetching translations.
	if result.Title != "Original Japanese Title" {
		t.Fatalf("Title = %q, want %q", result.Title, "Original Japanese Title")
	}
	if result.TitleLanguage != "ja" || result.TitleIsFallback || result.OriginalLanguage != "ja" {
		t.Fatalf("native title metadata = (%q, %v, %q)", result.TitleLanguage, result.TitleIsFallback, result.OriginalLanguage)
	}
	if translationCalls.Load() != 0 {
		t.Fatalf("translation endpoint called %d times, want 0", translationCalls.Load())
	}
}

func TestGetSeriesMetadata_UsesEmbeddedTranslationsWithoutDedicatedEndpoint(t *testing.T) {
	t.Parallel()

	var translationCalls atomic.Int32
	server := newTranslationTestServer(t, &translationCalls)
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "100"},
		ContentType: "series",
		Language:    "en",
	})
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	// Should get English data from embedded translations.
	if result.Title != "English Series Title" {
		t.Fatalf("Title = %q, want %q", result.Title, "English Series Title")
	}
	if result.Overview != "English series overview" {
		t.Fatalf("Overview = %q, want %q", result.Overview, "English series overview")
	}
	// Should NOT call the dedicated translation endpoint since embedded data was sufficient.
	if translationCalls.Load() != 0 {
		t.Fatalf("dedicated translation endpoint called %d times, want 0 (embedded was sufficient)", translationCalls.Load())
	}
}

func TestGetEpisodes_TranslatesViaBulkEndpoint(t *testing.T) {
	t.Parallel()

	var translationCalls atomic.Int32
	server := newTranslationTestServer(t, &translationCalls)
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	episodes, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{
		ProviderIDs:  map[string]string{"tvdb": "100"},
		SeasonNumber: 1,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("GetEpisodes() error = %v", err)
	}
	if len(episodes) != 3 {
		t.Fatalf("len(episodes) = %d, want 3", len(episodes))
	}

	// Verify all episodes were translated.
	for i, ep := range episodes {
		wantTitle := []string{"English Ep 1", "English Ep 2", "English Ep 3"}[i]
		wantOverview := []string{"EN overview 1", "EN overview 2", "EN overview 3"}[i]
		if ep.Title != wantTitle {
			t.Errorf("episodes[%d].Title = %q, want %q", i, ep.Title, wantTitle)
		}
		if ep.Overview != wantOverview {
			t.Errorf("episodes[%d].Overview = %q, want %q", i, ep.Overview, wantOverview)
		}
	}

	// The bulk translated endpoint must be hit exactly once for the whole
	// season — not once per episode (the old N+1).
	if got := translationCalls.Load(); got != 1 {
		t.Fatalf("bulk translation calls = %d, want 1 (no per-episode N+1)", got)
	}
}

func TestGetEpisodes_SkipsTranslationWhenLanguageMatchesOriginal(t *testing.T) {
	t.Parallel()

	var translationCalls atomic.Int32
	server := newTranslationTestServer(t, &translationCalls)
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	episodes, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{
		ProviderIDs:  map[string]string{"tvdb": "100"},
		SeasonNumber: 1,
		Language:     "ja", // matches originalLanguage "jpn"
	})
	if err != nil {
		t.Fatalf("GetEpisodes() error = %v", err)
	}
	if len(episodes) != 3 {
		t.Fatalf("len(episodes) = %d, want 3", len(episodes))
	}
	// Should use original data.
	if episodes[0].Title != "Japanese Ep 1" {
		t.Fatalf("episodes[0].Title = %q, want %q", episodes[0].Title, "Japanese Ep 1")
	}
	if translationCalls.Load() != 0 {
		t.Fatalf("translation endpoint called %d times, want 0", translationCalls.Load())
	}
}

func TestGetEpisodes_PartialTranslationFailureKeepsOriginalData(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"token": "test-token"},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/series/100/episodes/official":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"series": map[string]any{"id": 100, "originalLanguage": "jpn"},
					"episodes": []map[string]any{
						{"id": 301, "name": "JP Ep 1", "overview": "JP ov 1", "number": 1, "seasonNumber": 1},
						{"id": 302, "name": "JP Ep 2", "overview": "JP ov 2", "number": 2, "seasonNumber": 1},
					},
				},
				"links": map[string]any{"next": nil},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/series/100/episodes/official/eng":
			// Translated bulk list: ep 301 is translated; ep 302 has no
			// translation (empty name/overview), so its original must be kept.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"series": map[string]any{"id": 100, "originalLanguage": "jpn"},
					"episodes": []map[string]any{
						{"id": 301, "name": "English Ep 1", "overview": "EN ov 1", "number": 1, "seasonNumber": 1},
						{"id": 302, "name": "", "overview": "", "number": 2, "seasonNumber": 1},
					},
				},
				"links": map[string]any{"next": nil},
			})

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	episodes, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{
		ProviderIDs:  map[string]string{"tvdb": "100"},
		SeasonNumber: 1,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("GetEpisodes() error = %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("len(episodes) = %d, want 2", len(episodes))
	}

	// Episode 1 should be translated.
	if episodes[0].Title != "English Ep 1" {
		t.Errorf("episodes[0].Title = %q, want %q", episodes[0].Title, "English Ep 1")
	}
	// Episode 2 should keep original data after translation failure.
	if episodes[1].Title != "JP Ep 2" {
		t.Errorf("episodes[1].Title = %q, want %q (original kept after failure)", episodes[1].Title, "JP Ep 2")
	}
	if episodes[1].Overview != "JP ov 2" {
		t.Errorf("episodes[1].Overview = %q, want %q (original kept after failure)", episodes[1].Overview, "JP ov 2")
	}
}

// TestGetEpisodes_CachesAcrossSeasons verifies the bulk endpoint is fetched once
// for a multi-season series even when the server requests each season
// separately — the cache prevents a full-series re-fetch per season.
func TestGetEpisodes_CachesAcrossSeasons(t *testing.T) {
	t.Parallel()

	var baseCalls, transCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success", "data": map[string]any{"token": "test-token"},
			})

		case r.URL.Path == "/series/100/episodes/official":
			baseCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"series": map[string]any{"id": 100, "originalLanguage": "jpn"},
					"episodes": []map[string]any{
						{"id": 301, "name": "JP S1E1", "number": 1, "seasonNumber": 1},
						{"id": 401, "name": "JP S2E1", "number": 1, "seasonNumber": 2},
					},
				},
				"links": map[string]any{"next": nil},
			})

		case r.URL.Path == "/series/100/episodes/official/eng":
			transCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"series": map[string]any{"id": 100, "originalLanguage": "jpn"},
					"episodes": []map[string]any{
						{"id": 301, "name": "EN S1E1", "number": 1, "seasonNumber": 1},
						{"id": 401, "name": "EN S2E1", "number": 1, "seasonNumber": 2},
					},
				},
				"links": map[string]any{"next": nil},
			})

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	for _, season := range []int{1, 2} {
		eps, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{
			ProviderIDs:  map[string]string{"tvdb": "100"},
			SeasonNumber: season,
			Language:     "en",
		})
		if err != nil {
			t.Fatalf("GetEpisodes(season=%d) error = %v", season, err)
		}
		if len(eps) != 1 {
			t.Fatalf("season %d: len(episodes) = %d, want 1", season, len(eps))
		}
		wantTitle := map[int]string{1: "EN S1E1", 2: "EN S2E1"}[season]
		if eps[0].Title != wantTitle {
			t.Errorf("season %d: Title = %q, want %q", season, eps[0].Title, wantTitle)
		}
	}

	// Despite two GetEpisodes calls, each bulk endpoint is fetched exactly once.
	if got := baseCalls.Load(); got != 1 {
		t.Errorf("base bulk calls = %d, want 1 (cache should serve season 2)", got)
	}
	if got := transCalls.Load(); got != 1 {
		t.Errorf("translated bulk calls = %d, want 1 (cache should serve season 2)", got)
	}
}

// TestGetEpisodes_Paginates verifies a series whose episodes span multiple pages
// is fully assembled across pages.
func TestGetEpisodes_Paginates(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success", "data": map[string]any{"token": "test-token"},
			})

		case r.URL.Path == "/series/100/episodes/official":
			page := r.URL.Query().Get("page")
			if page == "0" {
				next := "page1"
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": "success",
					"data": map[string]any{
						"series": map[string]any{"id": 100, "originalLanguage": "eng"},
						"episodes": []map[string]any{
							{"id": 301, "name": "Ep 1", "number": 1, "seasonNumber": 1},
						},
					},
					"links": map[string]any{"next": next},
				})
				return
			}
			// page 1 (final).
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"series": map[string]any{"id": 100, "originalLanguage": "eng"},
					"episodes": []map[string]any{
						{"id": 302, "name": "Ep 2", "number": 2, "seasonNumber": 1},
					},
				},
				"links": map[string]any{"next": nil},
			})

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	// Language "en" matches originalLanguage "eng" → base list only, two pages.
	eps, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{
		ProviderIDs:  map[string]string{"tvdb": "100"},
		SeasonNumber: 1,
		Language:     "en",
	})
	if err != nil {
		t.Fatalf("GetEpisodes() error = %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("len(episodes) = %d, want 2 (both pages assembled)", len(eps))
	}
	if eps[0].Title != "Ep 1" || eps[1].Title != "Ep 2" {
		t.Errorf("titles = [%q, %q], want [Ep 1, Ep 2]", eps[0].Title, eps[1].Title)
	}
}

func TestGetSeriesMetadataCarriesShowStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]any{"token": "test-token"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/series/100/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":       100,
					"name":     "Series",
					"overview": "Series overview",
					"status": map[string]any{
						"id":   1,
						"name": "Continuing",
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	result, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "100"},
		ContentType: "series",
	})
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}
	if result.ShowStatus != "Continuing" {
		t.Fatalf("ShowStatus = %q, want %q", result.ShowStatus, "Continuing")
	}
}

func TestProviderIdentityAndMovieSearchMetadata(t *testing.T) {
	t.Parallel()
	p := NewProvider()
	if p.Name() == "" || p.Slug() != "tvdb" || len(p.ForTypes()) == 0 {
		t.Fatalf("identity %#v %#v %#v", p.Name(), p.Slug(), p.ForTypes())
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "t"}})
		case r.URL.Path == "/movies/42/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id": 42, "name": "Film", "originalLanguage": "fra", "runtime": 120, "year": "2010",
					"image": "https://artworks.thetvdb.com/banners/movies/42.jpg",
					"originalCountry": "fra",
					"genres": []map[string]any{{"name": "Drama"}},
					"studios": []map[string]any{{"name": "Studio"}},
					"contentRatings": []map[string]any{{"country": "usa", "name": "R", "contentRating": "R"}},
					"remoteIds": []map[string]any{{"type": 2, "id": "tt42", "sourceName": "IMDB"}, {"type": 12, "id": "99", "sourceName": "TheMovieDB.com"}},
					"aliases": []map[string]any{{"language": "eng", "name": "English Film"}},
					"translations": map[string]any{
						"nameTranslations": []map[string]any{{"language": "eng", "name": "English Film"}},
						"overviewTranslations": []map[string]any{{"language": "eng", "overview": "English overview"}},
					},
					"characters": []map[string]any{
						{"id": 1, "name": "Hero", "peopleId": 7, "personName": "Actor", "peopleType": "Actor", "image": "https://x/a.jpg"},
						{"id": 2, "name": "", "peopleId": 8, "personName": "Director", "peopleType": "Director"},
						{"id": 3, "name": "", "peopleId": 9, "personName": "Writer", "peopleType": "Writer"},
					},
					"first_release": map[string]any{"date": "2010-01-02"},
				},
			})
		case r.URL.Path == "/movies/42/translations/eng":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"name": "English Film", "overview": "English overview", "language": "eng"}})
		case r.URL.Path == "/search/remoteid/tt42":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []map[string]any{{"movie": map[string]any{"id": 42}}}})
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p = NewProviderWithClient(client)

	meta, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "42"}, ContentType: "movie", Language: "en",
	})
	if err != nil || meta == nil || meta.Title != "English Film" {
		t.Fatalf("meta=%#v err=%v", meta, err)
	}
	if len(meta.People) < 2 || meta.ContentRating == "" {
		t.Fatalf("people/rating %#v", meta)
	}

	byRemote, err := p.Search(context.Background(), metadata.SearchQuery{
		ProviderIDs: map[string]string{"imdb": "tt42"}, ContentType: "movie", Language: "en",
	})
	if err != nil || len(byRemote) != 1 {
		t.Fatalf("remote search %#v err=%v", byRemote, err)
	}

	byID, err := p.Search(context.Background(), metadata.SearchQuery{
		ProviderIDs: map[string]string{"tvdb": "42"}, ContentType: "movie", Language: "en",
	})
	if err != nil || len(byID) != 1 {
		t.Fatalf("id search %#v err=%v", byID, err)
	}
}

func TestGetSeasonsWithTranslations(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "t"}})
		case r.URL.Path == "/series/100/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id": 100, "name": "JP", "originalLanguage": "jpn",
					"seasons": []map[string]any{
						{"id": 200, "number": 1, "image": "https://artworks.thetvdb.com/banners/s1.jpg", "type": map[string]any{"id": 1, "name": "Aired Order"}},
						{"id": 201, "number": 0, "type": map[string]any{"id": 2, "name": "DVD"}},
					},
				},
			})
		case r.URL.Path == "/seasons/200/translations/eng":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"name": "Season One", "overview": "ov", "language": "eng"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	seasons, err := NewProviderWithClient(client).GetSeasons(context.Background(), metadata.SeasonsRequest{
		ProviderIDs: map[string]string{"tvdb": "100"}, Language: "en",
	})
	if err != nil || len(seasons) != 1 || seasons[0].Title != "Season One" {
		t.Fatalf("seasons=%#v err=%v", seasons, err)
	}
	empty, err := NewProviderWithClient(client).GetSeasons(context.Background(), metadata.SeasonsRequest{})
	if err != nil || empty != nil {
		t.Fatalf("empty=%v err=%v", empty, err)
	}
	if _, err := NewProviderWithClient(client).GetSeasons(context.Background(), metadata.SeasonsRequest{ProviderIDs: map[string]string{"tvdb": "x"}}); err == nil {
		t.Fatal("expected invalid id")
	}
}

func TestSearchBySeriesIDAndHelpers(t *testing.T) {
	t.Parallel()
	server := newTranslationTestServer(t, nil)
	defer server.Close()
	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)
	results, err := p.Search(context.Background(), metadata.SearchQuery{
		ProviderIDs: map[string]string{"tvdb": "100"}, ContentType: "series", Language: "en",
	})
	if err != nil || len(results) != 1 {
		t.Fatalf("results=%#v err=%v", results, err)
	}
	if findContentRating(nil) != "" {
		t.Fatal("nil rating")
	}
	if got := findContentRating([]ContentRating{{Country: "usa", Name: "TV-14"}}); got != "TV-14" {
		t.Fatalf("usa rating = %q", got)
	}
	if got := findContentRating([]ContentRating{{Country: "gbr", Name: "15"}}); got != "15" {
		t.Fatalf("fallback rating = %q", got)
	}
	if findTranslationName(nil, "en") != "" || findTranslationOverview(nil, "en") != "" {
		t.Fatal("nil translations")
	}
	if len(convertCharacters(nil)) != 0 {
		t.Fatal("nil chars")
	}
	bio := findBiography([]Biography{{Language: "eng", Biography: "EN"}, {Language: "jpn", Biography: "JP"}}, "en")
	if bio != "EN" {
		t.Fatalf("bio=%q", bio)
	}
	if findBiography(nil, "en") != "" {
		t.Fatal("empty bio")
	}
}

func TestMovieNativeLanguageAndFirstReleaseYear(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "t"}})
		case r.URL.Path == "/movies/7/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"id": 7, "name": "English Movie", "originalLanguage": "eng", "runtime": 90,
				"image": "https://artworks.thetvdb.com/banners/m.jpg",
				"first_release": map[string]any{"date": "2015-06-01"},
				"artworks": []map[string]any{
					{"image": "https://artworks.thetvdb.com/banners/m.jpg", "type": 2, "score": 10, "width": 1, "height": 2},
					{"image": "https://artworks.thetvdb.com/banners/b.jpg", "type": 3, "score": 5, "width": 3, "height": 4},
					{"image": "https://artworks.thetvdb.com/banners/l.png", "type": 22, "score": 1, "width": 5, "height": 6},
					{"image": "https://artworks.thetvdb.com/banners/x.jpg", "type": 99, "score": 1},
				},
				"characters": []map[string]any{
					{"personName": "Guest", "peopleId": 1, "peopleType": "Guest Star", "name": "G"},
					{"personName": "Prod", "peopleId": 2, "peopleType": "Producer"},
					{"personName": "Skip", "peopleId": 3, "peopleType": "Unknown"},
				},
			}})
		case r.URL.Path == "/search/remoteid/99":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []map[string]any{
				{"series": map[string]any{"id": 100}},
			}})
		case r.URL.Path == "/search/remoteid/nm1":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []map[string]any{
				{"people": map[string]any{"id": 7}},
			}})
		case r.URL.Path == "/people/7/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"id": 7, "name": "P", "image": "https://x/p.jpg",
				"biographies": []map[string]any{{"language": "jpn", "biography": "JP"}, {"language": "eng", "biography": "EN"}},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)

	meta, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "7"}, ContentType: "movie", Language: "en",
	})
	if err != nil || meta == nil || meta.Year != 2015 {
		t.Fatalf("meta=%#v err=%v", meta, err)
	}
	imgs, err := p.GetImages(context.Background(), metadata.ImageRequest{
		ProviderIDs: map[string]string{"tvdb": "7"}, ContentType: "movie",
	})
	if err != nil || len(imgs) < 3 {
		t.Fatalf("imgs=%d err=%v", len(imgs), err)
	}
	// convertCharacters covered via metadata people
	if len(meta.People) < 2 {
		t.Fatalf("people=%#v", meta.People)
	}

	// findByRemoteID series path via tmdb search — will fail GetSeriesExtended (404) after finding id
	_, _ = p.Search(context.Background(), metadata.SearchQuery{
		ProviderIDs: map[string]string{"tmdb": "99"}, ContentType: "series",
	})

	person, err := p.GetPersonDetail(context.Background(), metadata.PersonDetailRequest{
		ProviderIDs: map[string]string{"imdb": "nm1"}, Language: "en",
	})
	if err != nil || person == nil || person.Bio != "EN" {
		t.Fatalf("person=%#v err=%v", person, err)
	}
	person2, err := p.GetPersonDetail(context.Background(), metadata.PersonDetailRequest{
		ProviderIDs: map[string]string{"tmdb": "99"}, // will use findPersonByRemoteID; people missing => nil
	})
	_ = person2
	_ = err

	empty, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{ContentType: "movie"})
	if err != nil || empty != nil {
		t.Fatalf("empty meta %v %v", empty, err)
	}
	unsupported, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "7"}, ContentType: "game",
	})
	if err != nil || unsupported != nil {
		t.Fatalf("unsupported %v %v", unsupported, err)
	}
	if _, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "x"}, ContentType: "movie",
	}); err == nil {
		t.Fatal("invalid id")
	}
	if _, err := p.Search(context.Background(), metadata.SearchQuery{
		ProviderIDs: map[string]string{"tvdb": "x"}, ContentType: "movie",
	}); err == nil {
		t.Fatal("invalid search id")
	}
	nilSearch, err := p.Search(context.Background(), metadata.SearchQuery{ContentType: "movie"})
	if err != nil || nilSearch != nil {
		t.Fatalf("nil search %v %v", nilSearch, err)
	}
	if got, ok := artworkTypeToImageType(2); !ok || got != metadata.ImagePoster {
		t.Fatal("poster type")
	}
	if _, ok := artworkTypeToImageType(99); ok {
		t.Fatal("unknown art")
	}
	if extractYear("") != 0 || extractYear("20") != 0 || extractYear("abcd") != 0 {
		t.Fatal("extractYear")
	}
	if resolvedTitleLanguage("", "eng", "A", "A") != "en" {
		t.Fatal("resolvedTitleLanguage")
	}
	if findBiography([]Biography{{Language: "jpn", Biography: "JP"}}, "") != "JP" {
		t.Fatal("biography fallback")
	}
	if findBiography([]Biography{}, "en") != "" {
		t.Fatal("empty biographies")
	}
}

func TestMovieTranslationEndpointFallback(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "t"}})
		case r.URL.Path == "/movies/8/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"id": 8, "name": "日本語", "originalLanguage": "jpn", "year": "2020",
				// no embedded translations
			}})
		case r.URL.Path == "/movies/8/translations/eng":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"name": "Japanese Film", "overview": "English OV", "language": "eng",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	meta, err := NewProviderWithClient(client).GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "8"}, ContentType: "movie", Language: "en",
	})
	if err != nil || meta.Title != "Japanese Film" || meta.Overview != "English OV" {
		t.Fatalf("meta=%#v err=%v", meta, err)
	}
}

func TestSeriesMetadataRichAndMaxCast(t *testing.T) {
	t.Parallel()
	chars := make([]map[string]any, 0, 25)
	for i := 0; i < 22; i++ {
		chars = append(chars, map[string]any{
			"personName": "A", "peopleId": i + 1, "peopleType": "Actor", "name": "C",
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "t"}})
		case r.URL.Path == "/series/55/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"id": 55, "name": "Show", "overview": "ov", "originalLanguage": "eng", "year": "1999",
				"firstAired": "1999-01-01", "lastAired": "2000-01-01", "airsTime": "20:00",
				"image": "https://artworks.thetvdb.com/banners/p.jpg",
				"status": map[string]any{"name": "Ended"},
				"originalNetwork": map[string]any{"name": "NBC"},
				"genres": []map[string]any{{"name": "Drama"}},
				"contentRatings": []map[string]any{{"country": "usa", "name": "TV-14"}},
				"aliases": []map[string]any{{"language": "fra", "name": "Émission"}},
				"remoteIds": []map[string]any{{"type": 2, "id": "tt55", "sourceName": "IMDB"}},
				"seasons": []map[string]any{
					{"id": 1, "number": 1, "type": map[string]any{"id": 1}},
					{"id": 2, "number": 0, "type": map[string]any{"id": 1}},
				},
				"characters": chars,
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	meta, err := NewProviderWithClient(client).GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "55"}, ContentType: "series", Language: "en",
	})
	if err != nil || meta == nil {
		t.Fatalf("err=%v", err)
	}
	if meta.SeasonCount != 1 || meta.Networks[0] != "NBC" || meta.ContentRating != "TV-14" {
		t.Fatalf("meta=%#v", meta)
	}
	if len(meta.People) != maxCast {
		t.Fatalf("cast len=%d", len(meta.People))
	}
}

func TestFindByRemoteIDMovieAndEmpty(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "t"}})
		case r.URL.Path == "/search/remoteid/ttmovie":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []map[string]any{
				{"movie": map[string]any{"id": 42}},
			}})
		case r.URL.Path == "/search/remoteid/missing":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []any{}})
		case strings.HasPrefix(r.URL.Path, "/movies/42/extended"):
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"id": 42, "name": "M", "originalLanguage": "eng", "year": "2001",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)
	results, err := p.Search(context.Background(), metadata.SearchQuery{
		ProviderIDs: map[string]string{"imdb": "ttmovie"}, ContentType: "movie",
	})
	if err != nil || len(results) != 1 {
		t.Fatalf("results=%#v err=%v", results, err)
	}
	empty, err := p.Search(context.Background(), metadata.SearchQuery{
		ProviderIDs: map[string]string{"imdb": "missing"}, ContentType: "movie",
	})
	if err != nil || empty != nil {
		t.Fatalf("empty=%v err=%v", empty, err)
	}
}

func TestFindTranslationOverviewFallbacks(t *testing.T) {
	t.Parallel()
	td := &TranslationData{OverviewTranslations: []Translation{
		{Language: "deu", Overview: ""},
		{Language: "spa", Overview: "ES", IsPrimary: true},
		{Language: "fra", Overview: "FR"},
	}}
	if got := findTranslationOverview(td, "en"); got != "ES" {
		t.Fatalf("primary fallback = %q", got)
	}
	td2 := &TranslationData{OverviewTranslations: []Translation{
		{Language: "deu", Overview: ""},
		{Language: "fra", Overview: "FR"},
	}}
	if got := findTranslationOverview(td2, "en"); got != "FR" {
		t.Fatalf("first non-empty = %q", got)
	}
	if findTranslationOverview(&TranslationData{}, "en") != "" {
		t.Fatal("empty")
	}
	if findTranslationName(&TranslationData{NameTranslations: []Translation{{Language: "eng", Name: "N"}}}, "en") != "N" {
		t.Fatal("name match")
	}
}

func TestSeriesTranslationEndpointFallbackAndSeasonTranslationWarn(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "t"}})
		case r.URL.Path == "/series/66/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"id": 66, "name": "日本語", "overview": "JP ov", "originalLanguage": "jpn", "year": "2000",
				"originalCountry": "jpn",
				"seasons": []map[string]any{{"id": 9, "number": 1, "type": map[string]any{"id": 1}}},
			}})
		case r.URL.Path == "/series/66/translations/eng":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"name": "English", "overview": "EN ov", "language": "eng",
			}})
		case r.URL.Path == "/seasons/9/translations/eng":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "missing"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)
	meta, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{
		ProviderIDs: map[string]string{"tvdb": "66"}, ContentType: "series", Language: "en",
	})
	if err != nil || meta.Title != "English" || meta.Overview != "EN ov" || meta.Countries[0] != "jpn" {
		t.Fatalf("meta=%#v err=%v", meta, err)
	}
	seasons, err := p.GetSeasons(context.Background(), metadata.SeasonsRequest{
		ProviderIDs: map[string]string{"tvdb": "66"}, Language: "en",
	})
	if err != nil || len(seasons) != 1 {
		t.Fatalf("seasons=%#v err=%v", seasons, err)
	}
}

func TestGetPersonDetailErrorsAndImagesEmpty(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "t"}})
		case r.URL.Path == "/search/remoteid/bad":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "bad"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(1000)
	client.SetBaseURL(server.URL)
	p := NewProviderWithClient(client)
	if _, err := p.GetPersonDetail(context.Background(), metadata.PersonDetailRequest{
		ProviderIDs: map[string]string{"tvdb": "x"},
	}); err == nil {
		t.Fatal("invalid person id")
	}
	if _, err := p.GetPersonDetail(context.Background(), metadata.PersonDetailRequest{
		ProviderIDs: map[string]string{"imdb": "bad"},
	}); err == nil {
		t.Fatal("remote person error")
	}
	imgs, err := p.GetImages(context.Background(), metadata.ImageRequest{})
	if err != nil || imgs != nil {
		t.Fatalf("empty images %v %v", imgs, err)
	}
	if _, err := p.GetImages(context.Background(), metadata.ImageRequest{ProviderIDs: map[string]string{"tvdb": "x"}}); err == nil {
		t.Fatal("invalid image id")
	}
	eps, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{})
	if err != nil || eps != nil {
		t.Fatalf("empty eps %v %v", eps, err)
	}
	if _, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{ProviderIDs: map[string]string{"tvdb": "x"}}); err == nil {
		t.Fatal("invalid ep id")
	}
}
