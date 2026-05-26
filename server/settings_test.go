package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestSettings_SaveLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Missing file → zero settings, no error.
	got, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings (absent): %v", err)
	}
	if got.Port != 0 {
		t.Errorf("Port = %d, want 0 when no file exists", got.Port)
	}

	if err := SaveSettings(Settings{Port: 9123}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	got, err = LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got.Port != 9123 {
		t.Errorf("Port = %d, want 9123", got.Port)
	}
}

func TestResolveAddr(t *testing.T) {
	t.Run("not configurable ignores server.json", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		if err := SaveSettings(Settings{Port: 9000}); err != nil {
			t.Fatal(err)
		}
		s := &Server{logger: quietLogger(), portConfigurable: false, opts: Options{Addr: "0.0.0.0:8080"}}
		if got := s.resolveAddr(); got != "0.0.0.0:8080" {
			t.Errorf("resolveAddr = %q, want %q", got, "0.0.0.0:8080")
		}
	})

	t.Run("configurable uses persisted port", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		if err := SaveSettings(Settings{Port: 9000}); err != nil {
			t.Fatal(err)
		}
		s := &Server{logger: quietLogger(), portConfigurable: true}
		if got := s.resolveAddr(); got != "0.0.0.0:9000" {
			t.Errorf("resolveAddr = %q, want %q", got, "0.0.0.0:9000")
		}
	})

	t.Run("configurable defaults when unset", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		s := &Server{logger: quietLogger(), portConfigurable: true}
		if got := s.resolveAddr(); got != "0.0.0.0:8080" {
			t.Errorf("resolveAddr = %q, want %q", got, "0.0.0.0:8080")
		}
	})
}

func TestLanURLs(t *testing.T) {
	t.Run("wildcard host includes localhost", func(t *testing.T) {
		urls := lanURLs("0.0.0.0:8080")
		if !slices.Contains(urls, "http://localhost:8080") {
			t.Errorf("urls = %v, want to contain http://localhost:8080", urls)
		}
	})

	t.Run("specific host returns just that host", func(t *testing.T) {
		urls := lanURLs("192.168.1.5:8080")
		want := []string{"http://192.168.1.5:8080"}
		if !slices.Equal(urls, want) {
			t.Errorf("urls = %v, want %v", urls, want)
		}
	})

	t.Run("loopback host returns just that host", func(t *testing.T) {
		urls := lanURLs("127.0.0.1:9000")
		want := []string{"http://127.0.0.1:9000"}
		if !slices.Equal(urls, want) {
			t.Errorf("urls = %v, want %v", urls, want)
		}
	})
}

// postPort drives handleSetPort with a raw JSON body and returns the recorder.
func postPort(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/settings/port", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleSetPort(rec, req)
	return rec
}

func TestHandleSetPort_NotConfigurable(t *testing.T) {
	s := &Server{logger: quietLogger(), portConfigurable: false}
	rec := postPort(t, s, `{"port":9000}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleSetPort_OutOfRange(t *testing.T) {
	s := &Server{logger: quietLogger(), portConfigurable: true, restartCh: make(chan struct{}, 1)}
	s.setAddr("0.0.0.0:8080")
	rec := postPort(t, s, `{"port":80}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetPort_UnchangedIsNoOp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := &Server{logger: quietLogger(), portConfigurable: true, restartCh: make(chan struct{}, 1)}
	s.setAddr("0.0.0.0:8080")

	rec := postPort(t, s, `{"port":8080}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"restarting":false`) {
		t.Errorf("body = %q, want restarting:false", rec.Body.String())
	}
	// No restart should have been queued for an unchanged port.
	select {
	case <-s.restartCh:
		t.Error("restart was signalled for an unchanged port")
	default:
	}
}
