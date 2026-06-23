package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newAuthedActivityReq(t *testing.T, target string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	ctx := context.WithValue(req.Context(), tokenCtxKey, "tok")
	return req.WithContext(ctx)
}

// The activity report is design-scope only (the notifications feed is
// first-party-gated; the report is sourced from GraphQL). These cover the
// request validation that short-circuits before any upstream call; the
// GraphQL-to-report mapping is covered by api.TestDesignEventsFromDetails.
func TestHandleActivityReport_Validation(t *testing.T) {
	s := &Server{logger: quietLogger()}
	cases := []struct {
		name, target string
	}{
		{"non-design scope rejected", "/api/activity/report?scope=hub&hubId=H1&id=I1"},
		{"missing hubId", "/api/activity/report?scope=design&id=I1"},
		{"missing id", "/api/activity/report?scope=design&hubId=H1"},
		{"bad from", "/api/activity/report?scope=design&hubId=H1&id=I1&from=notatime"},
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
