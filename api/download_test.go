package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "plain alphanumeric",
			input: "MyDesign",
			want:  "MyDesign",
		},
		{
			name:  "space and dot are allowed",
			input: "My Design v2.0",
			want:  "My Design v2.0",
		},
		{
			name:  "path traversal — slashes replaced, dots kept",
			input: "../../etc/passwd",
			want:  ".._.._etc_passwd",
		},
		{
			name:  "non-ASCII letters replaced",
			input: "Caractères Spéciaux",
			want:  "Caract_res Sp_ciaux",
		},
		{
			name:  "all slashes become underscores (TrimSpace does not strip _)",
			input: "////",
			want:  "____",
		},
		{
			name:  "leading and trailing whitespace trimmed",
			input: "  spaces  ",
			want:  "spaces",
		},
		{
			name:  "null byte replaced",
			input: "with\x00null",
			want:  "with_null",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeFilename(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRequestSTEPDerivative_Statuses(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   string
	}{
		{name: "pending", status: "PENDING", want: "PENDING"},
		{name: "success", status: "SUCCESS", want: "SUCCESS"},
		{name: "failed", status: "FAILED", want: "FAILED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sawCV bool
			status := tc.status
			srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
				if v, ok := req.Variables["componentVersionId"].(string); ok && v == "cv-1" {
					sawCV = true
				} else {
					t.Errorf("Variables[componentVersionId] = %v, want \"cv-1\"", req.Variables["componentVersionId"])
				}
				return testutil.GraphQLResponse{Data: map[string]any{
					"componentVersion": map[string]any{
						"derivatives": []any{
							map[string]any{
								"status":       status,
								"signedUrl":    "https://signed.example/file.stp",
								"outputFormat": "STEP",
							},
						},
					},
				}}
			})
			swapEndpoint(t, srv.URL)

			gotStatus, gotURL, err := RequestSTEPDerivative(context.Background(), "tok", "cv-1")
			if err != nil {
				t.Fatalf("RequestSTEPDerivative: %v", err)
			}
			if !sawCV {
				t.Errorf("handler did not see componentVersionId variable")
			}
			if gotStatus != tc.want {
				t.Errorf("status = %q, want %q", gotStatus, tc.want)
			}
			if gotURL != "https://signed.example/file.stp" {
				t.Errorf("signedURL = %q, want %q", gotURL, "https://signed.example/file.stp")
			}
		})
	}
}

func TestRequestSTEPDerivative_NoDerivative(t *testing.T) {
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		return testutil.GraphQLResponse{Data: map[string]any{
			"componentVersion": map[string]any{
				"derivatives": []any{},
			},
		}}
	})
	swapEndpoint(t, srv.URL)

	_, _, err := RequestSTEPDerivative(context.Background(), "tok", "cv-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no STEP derivative") {
		t.Errorf("error = %q, want substring \"no STEP derivative\"", err.Error())
	}
}

func TestDownloadFile_Streams(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 100*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// H2 regression guardrail — APS signed URLs are self-authenticated;
		// sending the bearer token would leak credentials to any host the
		// signed URL points at.
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization header leaked to download endpoint: %q (must be empty — H2 regression!)", got)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "out.stp")
	if err := DownloadFile(context.Background(), srv.URL+"/some/file.stp", dest); err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading destination: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("downloaded bytes (%d) do not match served body (%d)", len(got), len(body))
	}
}

func TestDownloadFile_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden by policy"))
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "should-not-exist.stp")
	err := DownloadFile(context.Background(), srv.URL+"/blocked", dest)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "403") {
		t.Errorf("error = %q, want substring \"403\"", msg)
	}
	if !strings.Contains(msg, "forbidden by policy") {
		t.Errorf("error = %q, want substring \"forbidden by policy\"", msg)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("destination file should not exist; stat err = %v", statErr)
	}
}

func TestStepDownloadPath_Sanitizes(t *testing.T) {
	prevHome, prevNow := userHomeDir, nowFunc
	t.Cleanup(func() { userHomeDir, nowFunc = prevHome, prevNow })

	fixedNow := time.Date(2030, 1, 2, 15, 4, 5, 0, time.UTC)
	nowFunc = func() time.Time { return fixedNow }

	cases := []struct {
		name string
		// homeFn is the userHomeDir override for this case.
		homeFn func() (string, error)
		input  string
		// wantPath, when non-empty, is the exact expected output.
		wantPath string
		// fallbackTemp marks the home-error case; we only assert prefix/suffix.
		fallbackTemp bool
	}{
		{
			name:     "plain name",
			homeFn:   func() (string, error) { return "/home/test", nil },
			input:    "My Design",
			wantPath: filepath.Join("/home/test", "Downloads", "My Design-20300102-150405.stp"),
		},
		{
			name:     "slashes replaced",
			homeFn:   func() (string, error) { return "/home/test", nil },
			input:    "design/with/slashes",
			wantPath: filepath.Join("/home/test", "Downloads", "design_with_slashes-20300102-150405.stp"),
		},
		{
			name:     "empty falls back to design",
			homeFn:   func() (string, error) { return "/home/test", nil },
			input:    "",
			wantPath: filepath.Join("/home/test", "Downloads", "design-20300102-150405.stp"),
		},
		{
			name:         "home error falls back to TempDir",
			homeFn:       func() (string, error) { return "", errors.New("no home") },
			input:        "Widget",
			fallbackTemp: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userHomeDir = tc.homeFn
			got := StepDownloadPath(tc.input)
			if tc.fallbackTemp {
				wantPrefix := os.TempDir() + string(os.PathSeparator)
				wantSuffix := "-20300102-150405.stp"
				if !strings.HasPrefix(got, wantPrefix) {
					t.Errorf("StepDownloadPath(%q) = %q, want prefix %q", tc.input, got, wantPrefix)
				}
				if !strings.HasSuffix(got, wantSuffix) {
					t.Errorf("StepDownloadPath(%q) = %q, want suffix %q", tc.input, got, wantSuffix)
				}
				return
			}
			if got != tc.wantPath {
				t.Errorf("StepDownloadPath(%q) = %q, want %q", tc.input, got, tc.wantPath)
			}
		})
	}
}
