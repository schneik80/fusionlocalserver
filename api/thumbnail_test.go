package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

func TestGetThumbnail_Statuses(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   string
	}{
		{name: "pending", status: "PENDING", want: "PENDING"},
		{name: "success", status: "SUCCESS", want: "SUCCESS"},
		{name: "failed", status: "FAILED", want: "FAILED"},
		{name: "lowercase normalised", status: "success", want: "SUCCESS"},
		{name: "empty maps to failed", status: "", want: "FAILED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sawCV bool
			srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
				if v, ok := req.Variables["componentVersionId"].(string); ok && v == "cv-1" {
					sawCV = true
				} else {
					t.Errorf("Variables[componentVersionId] = %v, want \"cv-1\"", req.Variables["componentVersionId"])
				}
				return testutil.GraphQLResponse{Data: map[string]any{
					"componentVersion": map[string]any{
						"thumbnail": map[string]any{
							"status":    tc.status,
							"signedUrl": "https://signed.example/thumb.png",
						},
					},
				}}
			})
			swapEndpoint(t, srv.URL)

			gotStatus, gotURL, err := GetThumbnail(context.Background(), "tok", "cv-1")
			if err != nil {
				t.Fatalf("GetThumbnail: %v", err)
			}
			if !sawCV {
				t.Errorf("handler did not see componentVersionId variable")
			}
			if gotStatus != tc.want {
				t.Errorf("status = %q, want %q", gotStatus, tc.want)
			}
			if gotURL != "https://signed.example/thumb.png" {
				t.Errorf("signedURL = %q, want %q", gotURL, "https://signed.example/thumb.png")
			}
		})
	}
}

func TestClassifyAndThumbnail(t *testing.T) {
	t.Run("assembly with ready thumbnail", func(t *testing.T) {
		srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
			return testutil.GraphQLResponse{Data: map[string]any{
				"componentVersion": map[string]any{
					"occurrences": map[string]any{
						"results": []any{map[string]any{"id": "occ-1"}},
					},
					"thumbnail": map[string]any{
						"status":    "SUCCESS",
						"signedUrl": "https://signed.example/t.png",
					},
				},
			}}
		})
		swapEndpoint(t, srv.URL)

		isAsm, status, url, err := ClassifyAndThumbnail(context.Background(), "tok", "cv-1")
		if err != nil {
			t.Fatalf("ClassifyAndThumbnail: %v", err)
		}
		if !isAsm {
			t.Error("isAssembly = false, want true (one occurrence)")
		}
		if status != "SUCCESS" {
			t.Errorf("status = %q, want SUCCESS", status)
		}
		if url != "https://signed.example/t.png" {
			t.Errorf("url = %q", url)
		}
	})

	t.Run("part with empty thumbnail maps to FAILED", func(t *testing.T) {
		srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
			return testutil.GraphQLResponse{Data: map[string]any{
				"componentVersion": map[string]any{
					"occurrences": map[string]any{"results": []any{}},
					"thumbnail":   map[string]any{"status": "", "signedUrl": ""},
				},
			}}
		})
		swapEndpoint(t, srv.URL)

		isAsm, status, _, err := ClassifyAndThumbnail(context.Background(), "tok", "cv-1")
		if err != nil {
			t.Fatalf("ClassifyAndThumbnail: %v", err)
		}
		if isAsm {
			t.Error("isAssembly = true, want false (no occurrences)")
		}
		if status != "FAILED" {
			t.Errorf("status = %q, want FAILED (empty normalised)", status)
		}
	})
}

func TestFetchThumbnailImage(t *testing.T) {
	body := []byte("\x89PNG-fake-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Signed URLs are self-authenticated — the bearer must NOT be attached.
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization leaked to image fetch: %q", got)
		}
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	data, ctype, err := FetchThumbnailImage(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchThumbnailImage: %v", err)
	}
	if string(data) != string(body) {
		t.Errorf("data = %q, want %q", data, body)
	}
	if ctype != "image/png" {
		t.Errorf("ctype = %q, want image/png", ctype)
	}
}

func TestFetchThumbnailImage_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("denied"))
	}))
	defer srv.Close()

	if _, _, err := FetchThumbnailImage(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error for HTTP 403, got nil")
	}
}

func TestGetThumbnail_EmptyComponentVersionID(t *testing.T) {
	_, _, err := GetThumbnail(context.Background(), "tok", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty componentVersionID") {
		t.Errorf("error = %q, want substring \"empty componentVersionID\"", err.Error())
	}
}
