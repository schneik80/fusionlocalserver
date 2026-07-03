package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseOSSObjectURN(t *testing.T) {
	cases := []struct {
		urn, bucket, object string
		ok                  bool
	}{
		{"urn:adsk.objects:os.object:wip.dm.prod/abc123.md", "wip.dm.prod", "abc123.md", true},
		{"urn:adsk.objects:os.object:bucket/nested/key.md", "bucket", "nested/key.md", true},
		{"urn:adsk.objects:os.object:noslash", "", "", false},
		{"not-a-storage-urn", "", "", false},
	}
	for _, c := range cases {
		b, o, ok := parseOSSObjectURN(c.urn)
		if ok != c.ok || b != c.bucket || o != c.object {
			t.Errorf("parseOSSObjectURN(%q) = (%q,%q,%v), want (%q,%q,%v)", c.urn, b, o, ok, c.bucket, c.object, c.ok)
		}
	}
}

// TestListWikiPages walks the full read path (top folders → root contents →
// Wiki contents) against a stubbed Data Management API and checks that only the
// .md items are surfaced, with their metadata mapped through.
func TestListWikiPages(t *testing.T) {
	const rootID = "urn:adsk.wipprod:fs.folder:co.root"
	const wikiID = "urn:adsk.wipprod:fs.folder:co.wiki"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "topFolders"):
			w.Write([]byte(`{"data":[{"type":"folders","id":"` + rootID + `","attributes":{"name":"Root"}}]}`))
		case strings.Contains(r.URL.Path, "co.root"):
			w.Write([]byte(`{"data":[
				{"type":"folders","id":"` + wikiID + `","attributes":{"displayName":"Wiki"}},
				{"type":"folders","id":"urn:x:other","attributes":{"name":"Designs"}}
			]}`))
		case strings.Contains(r.URL.Path, "co.wiki"):
			w.Write([]byte(`{"data":[
				{"type":"items","id":"urn:adsk.wipprod:dm.lineage:p1","attributes":{"displayName":"Getting Started.md","lastModifiedTime":"2024-01-02T03:04:05+00:00","lastModifiedUserName":"Alice"},"relationships":{"tip":{"data":{"id":"urn:adsk.wipprod:vf.v1"}}}},
				{"type":"items","id":"urn:adsk.wipprod:dm.lineage:p2","attributes":{"displayName":"notes.txt"}},
				{"type":"folders","id":"urn:x:sub","attributes":{"name":"images"}}
			]}`))
		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer srv.Close()
	defer dmBaseURLForTest(srv.URL)()

	pages, err := ListWikiPages(context.Background(), "tok", "b.hub", "b.proj")
	if err != nil {
		t.Fatalf("ListWikiPages: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1: %+v", len(pages), pages)
	}
	p := pages[0]
	if p.Name != "Getting Started.md" {
		t.Errorf("name = %q, want %q", p.Name, "Getting Started.md")
	}
	if p.ItemID != "urn:adsk.wipprod:dm.lineage:p1" {
		t.Errorf("itemId = %q", p.ItemID)
	}
	if p.TipVersion != "urn:adsk.wipprod:vf.v1" {
		t.Errorf("tipVersion = %q", p.TipVersion)
	}
	if p.ModifiedBy != "Alice" {
		t.Errorf("modifiedBy = %q", p.ModifiedBy)
	}
}

// TestListWikiPagesNoFolder verifies an absent Wiki folder yields an empty slice
// (not an error): a project simply has no pages until one is published.
func TestListWikiPagesNoFolder(t *testing.T) {
	const rootID = "urn:adsk.wipprod:fs.folder:co.root"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "topFolders"):
			w.Write([]byte(`{"data":[{"type":"folders","id":"` + rootID + `","attributes":{"name":"Root"}}]}`))
		case strings.Contains(r.URL.Path, "co.root"):
			w.Write([]byte(`{"data":[{"type":"folders","id":"urn:x:designs","attributes":{"name":"Designs"}}]}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer srv.Close()
	defer dmBaseURLForTest(srv.URL)()

	pages, err := ListWikiPages(context.Background(), "tok", "b.hub", "b.proj")
	if err != nil {
		t.Fatalf("ListWikiPages: %v", err)
	}
	if len(pages) != 0 {
		t.Fatalf("got %d pages, want 0", len(pages))
	}
}
