package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetDesignBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"item": map[string]any{
					"__typename": "DesignItem",
					"name":       "Widget",
					"binary":     map[string]any{"id": "urn:adsk.wipprod:fs.file:vf.abc?version=5"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	restore := SetGraphqlEndpointForTesting(srv.URL)
	defer restore()

	b, err := GetDesignBinary(context.Background(), "tok", "hub", "item")
	if err != nil {
		t.Fatalf("GetDesignBinary: %v", err)
	}
	if b.VersionURN != "urn:adsk.wipprod:fs.file:vf.abc?version=5" {
		t.Errorf("VersionURN=%q", b.VersionURN)
	}
}

func TestGetDesignBinary_NoBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"item":{"__typename":"DesignItem","name":"X","binary":null}}}`))
	}))
	defer srv.Close()
	restore := SetGraphqlEndpointForTesting(srv.URL)
	defer restore()

	if _, err := GetDesignBinary(context.Background(), "tok", "hub", "item"); err == nil {
		t.Fatal("expected an error when binary is null")
	}
}

func TestOSSSignedDownloadURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"status":"complete","url":"https://s3.example/signed?x=1"}`))
	}))
	defer srv.Close()
	old := dmBaseURLForTest(srv.URL)
	defer old()

	u, err := OSSSignedDownloadURL(context.Background(), "tok", "urn:adsk.objects:os.object:wip.dm.prod/abc123.f3d")
	if err != nil {
		t.Fatalf("OSSSignedDownloadURL: %v", err)
	}
	if u != "https://s3.example/signed?x=1" {
		t.Errorf("url=%q", u)
	}
	if wantSub := "/oss/v2/buckets/wip.dm.prod/objects/abc123.f3d/signeds3download"; gotPath != wantSub {
		t.Errorf("path=%q want %q", gotPath, wantSub)
	}
}
