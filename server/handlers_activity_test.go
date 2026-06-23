package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/schneik80/fusionlocalserver/api"
)

const activityFeedFixture = `{
  "totalObjects": "2",
  "objects": [
    {"@type":"wipDioWidgetObject","type":"DATA","id":"D1","permalinkId":"D1","displayTitle":"One","fileType":"f3d",
     "creationTime":"1781808652000","changeTime":"1782168050000","version":"2","lineageUrn":"la","parentFolderUrn":"co.F1",
     "lastActivity":{"time":"1782168050000","accountId":"u1","displayName":"Alice"},
     "publishedTo":{"type":"Group","id":"P1","publishedToName":"Proj One"},
     "hub":{"name":"IMA","hubId":"imallc","forgeId":"a.YnVzaW5lc3M6aW1hbGxj"},"views":{"views":"3"}},
    {"@type":"wipDioWidgetObject","type":"DATA","id":"D2","permalinkId":"D2","displayTitle":"Two","fileType":"f3d",
     "creationTime":"1781800000000","changeTime":"1781820000000","version":"1","lineageUrn":"lb","parentFolderUrn":"co.F2",
     "lastActivity":{"time":"1781820000000","accountId":"u2","displayName":"Bob"},
     "publishedTo":{"type":"Group","id":"P2","publishedToName":"Proj Two"},
     "hub":{"name":"IMA","hubId":"imallc","forgeId":"a.YnVzaW5lc3M6aW1hbGxj"}}
  ],
  "links": {}
}`

func newAuthedActivityReq(t *testing.T, target string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	ctx := context.WithValue(req.Context(), tokenCtxKey, "tok")
	return req.WithContext(ctx)
}

func TestHandleActivityReport_HubScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(activityFeedFixture))
	}))
	defer srv.Close()
	defer api.SetFeedBaseURLForTesting(srv.URL)()

	s := &Server{logger: quietLogger()}
	rec := httptest.NewRecorder()
	s.handleActivityReport(rec, newAuthedActivityReq(t, "/api/activity/report?hub=imallc&scope=hub&bucket=day"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var rep ActivityReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rep.Scope != "hub" || rep.Bucket != "day" {
		t.Errorf("scope/bucket = %q/%q", rep.Scope, rep.Bucket)
	}
	if rep.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, want 2", rep.TotalEvents)
	}
	if len(rep.Children) != 2 {
		t.Errorf("children = %d, want 2 projects", len(rep.Children))
	}
	if rep.ContributorCount != 2 {
		t.Errorf("ContributorCount = %d, want 2", rep.ContributorCount)
	}
	if rep.DesignCount != 2 || rep.VersionCount != 3 { // la:2 + lb:1
		t.Errorf("designs/versions = %d/%d, want 2/3", rep.DesignCount, rep.VersionCount)
	}
}

func TestHandleActivityReport_ProjectScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(activityFeedFixture))
	}))
	defer srv.Close()
	defer api.SetFeedBaseURLForTesting(srv.URL)()

	s := &Server{logger: quietLogger()}
	rec := httptest.NewRecorder()
	s.handleActivityReport(rec, newAuthedActivityReq(t, "/api/activity/report?hub=imallc&scope=project&id=P1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var rep ActivityReportDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &rep)
	if rep.TotalEvents != 1 || rep.ScopeName != "Proj One" {
		t.Errorf("project report = total %d name %q, want 1 / Proj One", rep.TotalEvents, rep.ScopeName)
	}
}

func TestHandleActivityReport_Validation(t *testing.T) {
	s := &Server{logger: quietLogger()}
	cases := []struct {
		name, target string
	}{
		{"missing hub", "/api/activity/report"},
		{"bad scope", "/api/activity/report?hub=imallc&scope=galaxy"},
		{"project without id", "/api/activity/report?hub=imallc&scope=project"},
		{"bad from", "/api/activity/report?hub=imallc&from=notatime"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.handleActivityReport(rec, newAuthedActivityReq(t, c.target))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400 (body %q)", c.name, rec.Code, rec.Body.String())
			}
		})
	}
}
