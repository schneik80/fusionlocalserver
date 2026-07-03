package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPublishWikiPageNewItem walks the full new-page upload sequence (resolve
// Wiki folder → no existing page → create storage → signed S3 PUT → finalize →
// create item) against a stubbed Data Management + OSS API.
func TestPublishWikiPageNewItem(t *testing.T) {
	const (
		rootID     = "urn:adsk.wipprod:fs.folder:co.root"
		wikiID     = "urn:adsk.wipprod:fs.folder:co.wiki"
		storageURN = "urn:adsk.objects:os.object:wip.dm.prod/obj123.md"
		itemURN    = "urn:adsk.wipprod:dm.lineage:new1"
		versionURN = "urn:adsk.wipprod:vf.new1?version=1"
	)
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.Contains(p, "signeds3upload"):
			if r.Method == http.MethodGet {
				w.Write([]byte(`{"uploadKey":"k1","urls":["` + base + `/s3put"]}`))
			} else {
				w.Write([]byte(`{}`))
			}
		case r.Method == http.MethodPut: // the signed S3 upload target
			w.WriteHeader(http.StatusOK)
		case strings.Contains(p, "topFolders"):
			w.Write([]byte(`{"data":[{"type":"folders","id":"` + rootID + `","attributes":{"name":"Root"}}]}`))
		case strings.Contains(p, "co.root"):
			w.Write([]byte(`{"data":[{"type":"folders","id":"` + wikiID + `","attributes":{"displayName":"Wiki"}}]}`))
		case strings.Contains(p, "co.wiki"):
			w.Write([]byte(`{"data":[]}`)) // no existing pages
		case strings.HasSuffix(p, "/storage"):
			w.Write([]byte(`{"data":{"id":"` + storageURN + `"}}`))
		case strings.HasSuffix(p, "/items"):
			w.Write([]byte(`{"data":{"id":"` + itemURN + `","relationships":{"tip":{"data":{"id":"` + versionURN + `"}}}}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, p)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer srv.Close()
	base = srv.URL
	defer dmBaseURLForTest(srv.URL)()

	page, err := PublishWikiPage(context.Background(), "tok", "b.hub", "b.proj", "", "getting-started", "# Hi\n", "", false)
	if err != nil {
		t.Fatalf("PublishWikiPage: %v", err)
	}
	if page.ItemID != itemURN {
		t.Errorf("itemId = %q, want %q", page.ItemID, itemURN)
	}
	if page.TipVersion != versionURN {
		t.Errorf("tipVersion = %q, want %q", page.TipVersion, versionURN)
	}
	if page.Name != "getting-started.md" {
		t.Errorf("name = %q", page.Name)
	}
}

// TestPublishWikiPageNewVersion verifies re-publishing an existing page adds a
// version via the project-level POST .../versions endpoint (not the non-existent
// items/{id}/versions), referencing the item by relationship.
func TestPublishWikiPageNewVersion(t *testing.T) {
	const (
		rootID     = "urn:adsk.wipprod:fs.folder:co.root"
		wikiID     = "urn:adsk.wipprod:fs.folder:co.wiki"
		storageURN = "urn:adsk.objects:os.object:wip.dm.prod/obj9.md"
		itemID     = "urn:adsk.wipprod:dm.lineage:existing"
		newVerURN  = "urn:adsk.wipprod:vf.existing?version=2"
	)
	var base string
	var versionsPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.Contains(p, "signeds3upload"):
			if r.Method == http.MethodGet {
				w.Write([]byte(`{"uploadKey":"k","urls":["` + base + `/s3put"]}`))
			} else {
				w.Write([]byte(`{}`))
			}
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case strings.Contains(p, "topFolders"):
			w.Write([]byte(`{"data":[{"type":"folders","id":"` + rootID + `","attributes":{"name":"Root"}}]}`))
		case strings.Contains(p, "co.root"):
			w.Write([]byte(`{"data":[{"type":"folders","id":"` + wikiID + `","attributes":{"displayName":"Wiki"}}]}`))
		case strings.HasSuffix(p, "/storage"):
			w.Write([]byte(`{"data":{"id":"` + storageURN + `"}}`))
		case strings.HasSuffix(p, "/versions"):
			versionsPath = p
			w.Write([]byte(`{"data":{"id":"` + newVerURN + `"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, p)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer srv.Close()
	base = srv.URL
	defer dmBaseURLForTest(srv.URL)()

	// force=true skips the tip conflict check; itemID set → version path.
	page, err := PublishWikiPage(context.Background(), "tok", "a.hub", "a.proj", itemID, "page", "# v2\n", "", true)
	if err != nil {
		t.Fatalf("PublishWikiPage: %v", err)
	}
	if page.TipVersion != newVerURN {
		t.Errorf("tipVersion = %q, want %q", page.TipVersion, newVerURN)
	}
	if strings.Contains(versionsPath, "/items/") {
		t.Errorf("version created at item-scoped path %q; must be project-level /versions", versionsPath)
	}
	if !strings.HasSuffix(versionsPath, "/projects/a.proj/versions") {
		t.Errorf("unexpected versions path %q", versionsPath)
	}
}

// TestPublishWikiPageConflict verifies a stale base (live tip moved past the
// draft's baseVersion) is reported as ErrWikiConflict rather than overwriting.
func TestPublishWikiPageConflict(t *testing.T) {
	const (
		rootID = "urn:adsk.wipprod:fs.folder:co.root"
		wikiID = "urn:adsk.wipprod:fs.folder:co.wiki"
		itemID = "urn:adsk.wipprod:dm.lineage:existing"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.Contains(p, "topFolders"):
			w.Write([]byte(`{"data":[{"type":"folders","id":"` + rootID + `","attributes":{"name":"Root"}}]}`))
		case strings.Contains(p, "co.root"):
			w.Write([]byte(`{"data":[{"type":"folders","id":"` + wikiID + `","attributes":{"displayName":"Wiki"}}]}`))
		case strings.Contains(p, "/tip"):
			// Live tip is v3 — ahead of the draft's base (v2).
			w.Write([]byte(`{"data":{"id":"urn:adsk.wipprod:vf.existing?version=3"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, p)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer srv.Close()
	defer dmBaseURLForTest(srv.URL)()

	_, err := PublishWikiPage(context.Background(), "tok", "b.hub", "b.proj", itemID, "page", "# edit\n",
		"urn:adsk.wipprod:vf.existing?version=2", false)
	if !errors.Is(err, ErrWikiConflict) {
		t.Fatalf("err = %v, want ErrWikiConflict", err)
	}
}
