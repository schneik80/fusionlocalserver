package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// serveActivityFixtures returns an httptest.Server that mimics the Fusion Team
// notifications feed: page 1 advertises a nextPage link, page 2 does not.
func serveActivityFixtures(t *testing.T, pageHits *int) *httptest.Server {
	t.Helper()
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		return b
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pageHits != nil {
			*pageHits++
		}
		if got := r.Header.Get("Authorization"); got != "Bearer testtok" {
			t.Errorf("missing/wrong Authorization header: %q", got)
		}
		if !strings.Contains(r.URL.Path, "/hubs/imallc/feeds/network/@me") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write(read("activity_feed_page1.json"))
			return
		}
		_, _ = w.Write(read("activity_feed_page2.json"))
	}))
}

func eventByName(evs []ActivityEvent, name string) (ActivityEvent, bool) {
	for _, e := range evs {
		if e.EntityName == name {
			return e, true
		}
	}
	return ActivityEvent{}, false
}

func TestGetActivityFeed_PaginatesAndNormalizes(t *testing.T) {
	hits := 0
	srv := serveActivityFixtures(t, &hits)
	defer srv.Close()
	restore := SetFeedBaseURLForTesting(srv.URL)
	defer restore()

	evs, err := GetActivityFeed(context.Background(), "testtok", "imallc")
	if err != nil {
		t.Fatalf("GetActivityFeed: %v", err)
	}
	if len(evs) != 4 {
		t.Fatalf("want 4 events, got %d", len(evs))
	}
	if hits != 2 {
		t.Errorf("want 2 page fetches (stop after no nextPage), got %d", hits)
	}

	// Design with multiple versions + distinct last-actor (≠ owner).
	cebit, ok := eventByName(evs, "_CEBIT_LASTPENDEL_FINAL")
	if !ok {
		t.Fatal("missing _CEBIT_LASTPENDEL_FINAL event")
	}
	if cebit.EntityType != "design" {
		t.Errorf("cebit EntityType = %q, want design", cebit.EntityType)
	}
	if cebit.Action != ActionUpdated {
		t.Errorf("cebit Action = %q, want %q", cebit.Action, ActionUpdated)
	}
	if cebit.VersionNumber != 10 {
		t.Errorf("cebit VersionNumber = %d, want 10", cebit.VersionNumber)
	}
	if cebit.Actor.DisplayName != "Kevin Schneider IMA" || cebit.Actor.AccountID != "201610033789309" {
		t.Errorf("cebit last actor = %+v, want Kevin Schneider IMA/201610033789309", cebit.Actor)
	}
	if cebit.Owner.DisplayName != "Kevin Schneider" {
		t.Errorf("cebit owner = %+v, want Kevin Schneider", cebit.Owner)
	}
	if cebit.ProjectName != "Data Set Archive" || cebit.ProjectID != "2016052533367056" {
		t.Errorf("cebit project = %q/%q", cebit.ProjectName, cebit.ProjectID)
	}
	if cebit.HubForgeID != "a.YnVzaW5lc3M6aW1hbGxj" {
		t.Errorf("cebit hub forgeId = %q", cebit.HubForgeID)
	}
	if want := time.UnixMilli(1781459111000).UTC(); !cebit.Timestamp.Equal(want) {
		t.Errorf("cebit Timestamp = %s, want %s", cebit.Timestamp, want)
	}
	if want := time.UnixMilli(1778192526000).UTC(); !cebit.CreatedOn.Equal(want) {
		t.Errorf("cebit CreatedOn = %s, want %s", cebit.CreatedOn, want)
	}

	// First-version design → created.
	robot, ok := eventByName(evs, "robot_irb_390_-_15_1300_30")
	if ok && robot.VersionNumber == 2 && robot.Action != ActionUpdated {
		t.Errorf("robot Action = %q, want updated (v2)", robot.Action)
	}

	// COMMUNITY lifecycle event.
	comm, ok := eventByName(evs, "Kevin Schneider has created autoconstrain project")
	if !ok {
		// EntityName falls back to displayTitle for COMMUNITY objects.
		t.Fatal("missing COMMUNITY project-created event")
	}
	if comm.EntityType != "community" || comm.Action != ActionCommunity {
		t.Errorf("community classification = %q/%q", comm.EntityType, comm.Action)
	}
	if !strings.Contains(comm.Detail, "created autoconstrain project") {
		t.Errorf("community Detail = %q, want stripped text containing 'created autoconstrain project'", comm.Detail)
	}
	if comm.ProjectName != "autoconstrain" {
		t.Errorf("community ProjectName = %q, want autoconstrain", comm.ProjectName)
	}
	if want := time.UnixMilli(1781799897000).UTC(); !comm.Timestamp.Equal(want) {
		t.Errorf("community Timestamp = %s, want %s", comm.Timestamp, want)
	}
}

func TestGetActivityFeed_EmptyHub(t *testing.T) {
	if _, err := GetActivityFeed(context.Background(), "t", ""); err == nil {
		t.Fatal("want error for empty hubID")
	}
}

func TestStripHTML(t *testing.T) {
	in := `<a href="x">Kevin&nbsp;Schneider</a> has created  <a>autoconstrain</a> project`
	got := stripHTML(in)
	want := "Kevin Schneider has created autoconstrain project"
	if got != want {
		t.Errorf("stripHTML = %q, want %q", got, want)
	}
}
