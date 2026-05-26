package api

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedactSignedURLs(t *testing.T) {
	const secret = "https://cdn.example.com/derivative.png?sig=DEADBEEF&exp=1700000000"
	tests := []struct {
		name string
		in   string
		// wantContains must appear; wantAbsent must not (the secret value).
		wantContains []string
		secretLeaks  bool
	}{
		{
			name:         "redacts signedUrl value, keeps key and siblings",
			in:           `{"status":"SUCCESS","signedUrl":"` + secret + `","id":"abc"}`,
			wantContains: []string{`"signedUrl":"[redacted]"`, `"status":"SUCCESS"`, `"id":"abc"`},
		},
		{
			name:         "case-insensitive field name",
			in:           `{"signedURL":"` + secret + `"}`,
			wantContains: []string{`[redacted]`},
		},
		{
			name:         "multiple occurrences",
			in:           `[{"signedUrl":"` + secret + `"},{"signedUrl":"` + secret + `"}]`,
			wantContains: []string{`[redacted]`},
		},
		{
			name:         "no signed url is a no-op",
			in:           `{"status":"PENDING","url":"/local/path"}`,
			wantContains: []string{`"status":"PENDING"`, `"url":"/local/path"`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := redactSignedURLs(tc.in)
			if strings.Contains(got, secret) {
				t.Errorf("secret URL leaked through redaction:\n%s", got)
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("redacted output missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestDbgLog_GatedAndRedacted(t *testing.T) {
	const secret = "https://cdn.example.com/x.png?sig=SECRET"

	// Disabled: nothing is written.
	t.Cleanup(func() { EnableDebug(nil) })
	EnableDebug(nil)
	var off bytes.Buffer
	dbgSink = &off // sink set but disabled; dbgLog must still no-op
	dbgEnabled = false
	dbgLog("RESPONSE %s", `{"signedUrl":"`+secret+`"}`)
	if off.Len() != 0 {
		t.Fatalf("dbgLog wrote while disabled: %q", off.String())
	}

	// Enabled: writes, but redacts the signed URL.
	var on bytes.Buffer
	EnableDebug(&on)
	dbgLog("RESPONSE %s", `{"signedUrl":"`+secret+`"}`)
	out := on.String()
	if strings.Contains(out, secret) {
		t.Errorf("dbgLog leaked the signed URL: %q", out)
	}
	if !strings.Contains(out, "[redacted]") {
		t.Errorf("dbgLog did not redact: %q", out)
	}
}
