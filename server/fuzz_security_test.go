package server

// QA security: fuzzing + boundary suites for the JSON-body endpoints.
//
// These call handlers directly (bypassing requireAuth) so they exercise the
// parse/validate path the way an authenticated attacker would reach it. The
// invariant under fuzzing is resilience: the handler must never panic and must
// never emit a 5xx for client-supplied garbage — only the defined 4xx (or, for
// a well-formed-but-unauthenticated request, 401). Run the fuzzers with:
//
//	go test ./server/ -run xxx -fuzz FuzzPinsAddBody   -fuzztime 30s
//	go test ./server/ -run xxx -fuzz FuzzRollupBody     -fuzztime 30s
//
// Without -fuzz they execute as ordinary seed-corpus tests.

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setTempHomeFuzz redirects the pins on-disk store at a throwaway dir for the
// lifetime of a fuzz run. Uses os.Setenv (not t.Setenv) because fuzz targets
// run with parallelism, where t.Setenv panics.
func setTempHomeFuzz(f *testing.F) {
	f.Helper()
	home := f.TempDir()
	prev, had := os.LookupEnv("HOME")
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	f.Cleanup(func() {
		if had {
			os.Setenv("HOME", prev)
		} else {
			os.Unsetenv("HOME")
		}
	})
	if err := os.MkdirAll(filepath.Join(home, ".config", "fusionlocalserver"), 0700); err != nil {
		f.Fatalf("setup config dir: %v", err)
	}
}

// assertResilient is the shared fuzz invariant: a real HTTP status, never 5xx.
func assertResilient(t *testing.T, code int, body, input string) {
	t.Helper()
	if code >= 500 {
		t.Fatalf("5xx (%d) on client input %q -> %q", code, input, body)
	}
	if code < 100 || code > 599 {
		t.Fatalf("nonsense status %d on input %q", code, input)
	}
}

// FuzzPinsAddBody throws arbitrary bytes at the pin-create body. Malformed JSON
// must 400; a valid pin must 200; nothing may panic or 5xx. The 64 KiB
// MaxBytesReader cap means even a multi-MB blob is bounded, not buffered whole.
func FuzzPinsAddBody(f *testing.F) {
	setTempHomeFuzz(f)
	seeds := []string{
		`{"id":"x","kind":"design"}`,              // valid
		`{"id":"x","kind":`,                       // missing bracket / truncated
		`{"id":123,"kind":true}`,                  // type mismatches
		"{\"id\":\"x\x00y\",\"kind\":\"design\"}", // embedded null byte (invalid JSON control char)
		`[]`, `null`, `"`, `{}`, ``, // structural edge cases
		`{"id":"` + strings.Repeat("A", 70<<10) + `"}`, // over the cap
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body string) {
		s := &Server{logger: quietLogger()}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/pins?hubId=h1", strings.NewReader(body))
		s.handlePinsAdd(rec, req)
		assertResilient(t, rec.Code, rec.Body.String(), body)
		// Only the contract statuses are acceptable here.
		switch rec.Code {
		case http.StatusOK, http.StatusBadRequest:
		default:
			t.Fatalf("unexpected status %d for body %q", rec.Code, body)
		}
	})
}

// FuzzRollupBody fuzzes the activity-rollup body. Validation (size cap, required
// fields, child-count cap) runs before the token lookup, so a well-formed body
// reaches the 401 gate and garbage is rejected at 400 — never 5xx.
func FuzzRollupBody(f *testing.F) {
	seeds := []string{
		`{"hubId":"h","itemId":"i","childItemIds":["a","b"]}`,
		`{"hubId":"h","itemId":"i"`, // truncated
		`{"hubId":42}`,              // type mismatch
		`{"childItemIds":"not-an-array"}`,
		`{}`, `null`, ``,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body string) {
		s := &Server{logger: quietLogger()}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/activity/rollup", strings.NewReader(body))
		s.handleActivityRollup(rec, req)
		assertResilient(t, rec.Code, rec.Body.String(), body)
		switch rec.Code {
		case http.StatusBadRequest, http.StatusUnauthorized:
		default:
			t.Fatalf("unexpected status %d for body %q", rec.Code, body)
		}
	})
}

// --- Boundary tables (deterministic; run under plain `go test`) -------------

// TestSetPortBoundaries drives the integer boundary values the port endpoint
// must reject. It deliberately avoids any in-range *different* port so the
// handler never performs a real bind/save/restart — only the validation branch
// (out-of-range -> 400) and the unchanged no-op (-> 200) are exercised.
func TestSetPortBoundaries(t *testing.T) {
	cases := []struct {
		name string
		port string
		want int
	}{
		{"negative", "-1", http.StatusBadRequest},
		{"zero", "0", http.StatusBadRequest},
		{"privileged_80", "80", http.StatusBadRequest},
		{"just_below_min", "1023", http.StatusBadRequest},
		{"just_above_max", "65536", http.StatusBadRequest},
		{"int64_max", "9223372036854775807", http.StatusBadRequest},
		{"overflow_beyond_int64", "99999999999999999999", http.StatusBadRequest}, // decode error
		{"unchanged_noop", "8080", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			s := &Server{logger: quietLogger(), portConfigurable: true, restartCh: make(chan struct{}, 1)}
			s.setAddr("0.0.0.0:8080")
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/settings/port",
				strings.NewReader(`{"port":`+tc.port+`}`))
			s.handleSetPort(rec, req)
			if rec.Code != tc.want {
				t.Errorf("port=%s: status = %d, want %d (body %q)", tc.port, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
	_ = math.MaxInt64 // documents the intent of the int64_max case
}

// TestNullBytesAndLongStringsArePinData feeds null bytes and an oversized string
// through the pin path. Null bytes must be handled as opaque data (the hub id is
// sanitised for the on-disk filename), and an over-cap body must 400 rather than
// allocate unbounded memory.
func TestNullBytesAndLongStringsArePinData(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".config", "fusionlocalserver"), 0700); err != nil {
		t.Fatal(err)
	}
	s := &Server{logger: quietLogger()}

	t.Run("null_byte_in_hub_query", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/pins?hubId=h%001",
			strings.NewReader(`{"id":"x","kind":"design"}`))
		s.handlePinsAdd(rec, req)
		if rec.Code >= 500 {
			t.Fatalf("null byte in hubId caused %d: %q", rec.Code, rec.Body.String())
		}
	})

	t.Run("oversized_body_rejected", func(t *testing.T) {
		huge := `{"id":"x","kind":"design","name":"` + strings.Repeat("A", 1<<20) + `"}`
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/pins?hubId=h1", strings.NewReader(huge))
		s.handlePinsAdd(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("oversized body: status = %d, want 400", rec.Code)
		}
	})
}

// TestRollupChildCountBoundary confirms the fan-out cap holds exactly at the
// boundary: maxRollupChildren entries pass validation (reaching the 401 gate),
// one more is rejected at 400 before any work is scheduled.
func TestRollupChildCountBoundary(t *testing.T) {
	mkBody := func(n int) string {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = `"c"`
		}
		return `{"hubId":"h","itemId":"i","childItemIds":[` + strings.Join(ids, ",") + `]}`
	}
	s := &Server{logger: quietLogger()}

	// At the cap: passes validation, falls through to the (unauthenticated) 401.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/activity/rollup", strings.NewReader(mkBody(maxRollupChildren)))
	s.handleActivityRollup(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("at cap: status = %d, want 401 (validation should pass)", rec.Code)
	}

	// One over the cap: rejected at 400 before the token lookup.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/activity/rollup", strings.NewReader(mkBody(maxRollupChildren+1)))
	s.handleActivityRollup(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("over cap: status = %d, want 400", rec.Code)
	}
}
