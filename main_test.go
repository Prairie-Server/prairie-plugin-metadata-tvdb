package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	"github.com/prairie-server/prairie-plugin-metadata-tvdb/metadata"
	"github.com/prairie-server/prairie-plugin-metadata-tvdb/models"
	"github.com/prairie-server/prairie-plugin-metadata-tvdb/provider"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestResolveImageURL(t *testing.T) {
	ms := &metadataServer{}
	tests := []struct {
		name, path, variant, want string
	}{
		{"poster card", "banners/posters/81189-10.jpg", "card", "https://artworks.thetvdb.com/banners/posters/81189-10_t.jpg"},
		{"poster featured", "banners/posters/81189-10.jpg", "featured", "https://artworks.thetvdb.com/banners/posters/81189-10.jpg"},
		{"poster original", "banners/posters/81189-10.jpg", "original", "https://artworks.thetvdb.com/banners/posters/81189-10.jpg"},
		{"empty variant", "banners/posters/81189-10.jpg", "", "https://artworks.thetvdb.com/banners/posters/81189-10.jpg"},
		{"backdrop card", "banners/fanart/81189-5.jpg", "card", "https://artworks.thetvdb.com/banners/fanart/81189-5_t.jpg"},
		{"empty path", "", "card", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ms.ResolveImageURL(context.Background(), &pluginv1.ResolveImageURLRequest{
				Path: tt.path, Variant: tt.variant,
			})
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if resp.GetUrl() != tt.want {
				t.Fatalf("got %q, want %q", resp.GetUrl(), tt.want)
			}
		})
	}
}

func TestRuntimeServerConfigure_NoOp(t *testing.T) {
	server := &runtimeServer{provider: provider.NewProvider()}

	_, err := server.Configure(context.Background(), &pluginv1.ConfigureRequest{})
	if err != nil {
		t.Fatalf("Configure() returned error: %v", err)
	}

	p, err := server.providerForRequest()
	if err != nil {
		t.Fatalf("providerForRequest() returned error: %v", err)
	}
	if p == nil {
		t.Fatal("expected provider to be available")
	}
}

func TestPersonDetailRecordFromResult_CanonicalizesPhotoPath(t *testing.T) {
	record, err := personDetailRecordFromResult(&metadata.PersonDetailResult{
		Name:           "Sigourney Weaver",
		SortName:       "Weaver, Sigourney",
		Bio:            "English biography",
		BirthDate:      "1949-10-08",
		Birthplace:     "New York City, New York, USA",
		PhotoPath:      "https://artworks.thetvdb.com/banners/persons/321.jpg",
		PhotoThumbhash: "thumbhash-123",
		ProviderIDs: map[string]string{
			"tvdb": "321",
			"imdb": "nm0000244",
		},
	})
	if err != nil {
		t.Fatalf("personDetailRecordFromResult() error = %v", err)
	}
	if record.GetPhotoPath() != "tvdb://banners/persons/321.jpg" {
		t.Fatalf("record.PhotoPath = %q, want tvdb canonical path", record.GetPhotoPath())
	}
	if record.GetPhotoThumbhash() != "thumbhash-123" {
		t.Fatalf("record.PhotoThumbhash = %q, want thumbhash-123", record.GetPhotoThumbhash())
	}
	if record.GetProviderIds().AsMap()["tvdb"] != "321" {
		t.Fatalf("record.ProviderIds[tvdb] = %#v", record.GetProviderIds().AsMap()["tvdb"])
	}
}

func mustStruct(t *testing.T, value map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(value)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLoadManifestAndGetManifest(t *testing.T) {
	original := version
	version = "8.8.8-test"
	t.Cleanup(func() { version = original })
	manifest, err := loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.GetVersion() != "8.8.8-test" || len(manifest.GetChecksum()) != 64 {
		t.Fatalf("manifest version/checksum = %q %q", manifest.GetVersion(), manifest.GetChecksum())
	}
	rs := &runtimeServer{manifest: manifest, provider: provider.NewProvider()}
	resp, err := rs.GetManifest(context.Background(), &pluginv1.GetManifestRequest{})
	if err != nil || resp.GetManifest() != manifest {
		t.Fatalf("GetManifest err=%v", err)
	}
}

func TestMetadataServerRPCSurface(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"token": "t"}})
		case r.URL.Path == "/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []map[string]any{{
				"name": "Show", "year": "2001", "tvdb_id": "100", "overview": "ov",
				"image_url": "https://artworks.thetvdb.com/banners/poster.jpg",
				"primary_language": "eng",
			}}})
		case r.URL.Path == "/series/100/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"id": 100, "name": "Show", "overview": "ov", "originalLanguage": "eng", "year": "2001",
				"image": "https://artworks.thetvdb.com/banners/poster.jpg",
				"status": map[string]any{"name": "Ended"},
				"genres": []map[string]any{{"name": "Drama"}},
				"artworks": []map[string]any{
					{"id": 1, "image": "https://artworks.thetvdb.com/banners/poster.jpg", "type": 2, "score": 100, "width": 1000, "height": 1500},
					{"id": 2, "image": "https://artworks.thetvdb.com/banners/fanart.jpg", "type": 3, "score": 80, "width": 1920, "height": 1080},
					{"id": 3, "image": "https://artworks.thetvdb.com/banners/logo.png", "type": 22, "score": 50, "width": 800, "height": 400},
				},
				"seasons": []map[string]any{{"id": 200, "number": 1, "image": "https://artworks.thetvdb.com/banners/s1.jpg", "type": map[string]any{"id": 1}}},
				"characters": []map[string]any{{"personName": "Actor", "peopleId": 1, "peopleType": "Actor", "name": "Hero"}},
			}})
		case r.URL.Path == "/series/100/episodes/official":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"series": map[string]any{"id": 100, "originalLanguage": "eng"},
				"episodes": []map[string]any{{"id": 301, "name": "Pilot", "overview": "ep", "number": 1, "seasonNumber": 1, "runtime": 42, "image": "https://artworks.thetvdb.com/banners/still.jpg"}},
			}, "links": map[string]any{"next": nil}})
		case r.URL.Path == "/people/7/extended":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{
				"id": 7, "name": "Person", "image": "https://artworks.thetvdb.com/banners/persons/7.jpg",
				"birth": "1970-01-02", "birthPlace": "NY",
				"biographies": []map[string]any{{"language": "eng", "biography": "Bio"}},
				"remoteIds": []map[string]any{{"type": 2, "id": "nm7", "sourceName": "IMDB"}},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := provider.NewClient(1000)
	client.SetBaseURL(server.URL)
	ms := &metadataServer{runtime: &runtimeServer{provider: provider.NewProviderWithClient(client)}}

	search, err := ms.Search(context.Background(), &pluginv1.SearchMetadataRequest{Query: "Show", ItemType: "series", Language: "en"})
	if err != nil || len(search.GetResults()) != 1 {
		t.Fatalf("Search %#v err=%v", search, err)
	}
	meta, err := ms.GetMetadata(context.Background(), &pluginv1.GetMetadataRequest{ProviderId: "100", ItemType: "series", Language: "en"})
	if err != nil || meta.GetItem() == nil {
		t.Fatalf("GetMetadata %#v err=%v", meta, err)
	}
	if meta.GetItem().GetPosterPath() == "" || len(meta.GetItem().GetPeople()) == 0 {
		t.Fatalf("item fields %#v", meta.GetItem())
	}
	person, err := ms.GetPersonDetail(context.Background(), &pluginv1.GetPersonDetailRequest{ProviderIds: mustStruct(t, map[string]any{"tvdb": "7"})})
	if err != nil || person.GetPerson() == nil {
		t.Fatalf("person %#v err=%v", person, err)
	}
	seasons, err := ms.GetSeasons(context.Background(), &pluginv1.GetSeasonsRequest{SeriesProviderId: "100"})
	if err != nil || len(seasons.GetSeasons()) != 1 {
		t.Fatalf("seasons %#v err=%v", seasons, err)
	}
	episodes, err := ms.GetEpisodes(context.Background(), &pluginv1.GetEpisodesRequest{SeriesProviderId: "100", SeasonNumber: 1})
	if err != nil || len(episodes.GetEpisodes()) != 1 {
		t.Fatalf("episodes %#v err=%v", episodes, err)
	}
	images, err := ms.GetImages(context.Background(), &pluginv1.GetImagesRequest{ProviderId: "100", ItemType: "series"})
	if err != nil || len(images.GetImages()) < 2 {
		t.Fatalf("images %#v err=%v", images, err)
	}
	urls, err := ms.ResolveImageURLs(context.Background(), &pluginv1.ResolveImageURLsRequest{
		Paths: []string{"banners/posters/x.jpg", ""}, Variant: "card",
	})
	if err != nil || urls.GetUrls()["banners/posters/x.jpg"] == "" {
		t.Fatalf("urls %#v err=%v", urls, err)
	}
	nilPerson, err := ms.GetPersonDetail(context.Background(), &pluginv1.GetPersonDetailRequest{})
	if err != nil || nilPerson.GetPerson() != nil {
		t.Fatalf("nil person %#v err=%v", nilPerson, err)
	}
	nilMeta, err := ms.GetMetadata(context.Background(), &pluginv1.GetMetadataRequest{ItemType: "series"})
	if err != nil || (nilMeta != nil && nilMeta.GetItem() != nil) {
		t.Fatalf("nil meta %#v err=%v", nilMeta, err)
	}
}

func TestMainHelpers(t *testing.T) {
	if tvdbCanonicalPath("") != "" || tvdbThumbnailURL("noext") != "noext" {
		t.Fatal("path helpers")
	}
	aliases := aliasesToProto([]metadata.TitleAlias{{Title: " "}, {Title: "A", Language: "en", Kind: "alternate"}})
	if len(aliases) != 1 {
		t.Fatal(aliases)
	}
	if peopleToRecords(nil) != nil {
		t.Fatal("nil people")
	}
	people := peopleToRecords([]models.ItemPerson{{Person: models.Person{Name: "N", TvdbID: "1"}, Kind: models.PersonKindActor}})
	if len(people) != 1 {
		t.Fatal(people)
	}
	if stringMapFromStruct(nil) == nil {
		t.Fatal("nil map")
	}
	ids := providerIDsFromProto(mustStruct(t, map[string]any{"imdb": "tt1"}), "tvdb", "100")
	if ids["tvdb"] != "100" || ids["imdb"] != "tt1" {
		t.Fatal(ids)
	}
	_ = metadataRequestFromProto(&pluginv1.GetMetadataRequest{ProviderId: "1", ItemType: "movie", Language: "en", FilePath: "/x"}, "tvdb")
	_ = personDetailRequestFromProto(&pluginv1.GetPersonDetailRequest{Language: "en"})
	_ = seasonsRequestFromProto(&pluginv1.GetSeasonsRequest{SeriesProviderId: "1"}, "tvdb")
	_ = episodesRequestFromProto(&pluginv1.GetEpisodesRequest{SeriesProviderId: "1", SeasonNumber: 2}, "tvdb")
	_ = imageRequestFromProto(&pluginv1.GetImagesRequest{ProviderId: "1", ItemType: "movie"}, "tvdb")
	if s, err := stringStruct(nil); err != nil || s != nil {
		t.Fatal(s, err)
	}
	if s, err := stringStruct(map[string]string{"a": ""}); err != nil || s != nil {
		t.Fatal(s, err)
	}
	if structFromMap(nil) != nil {
		t.Fatal("structFromMap")
	}
	if ratingsStruct(metadata.Ratings{IMDB: 1, TMDB: 2, RTCritic: 3, RTAudience: 4}) == nil {
		t.Fatal("ratings")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = structFromMap(map[string]any{"bad": make(chan int)})
}
