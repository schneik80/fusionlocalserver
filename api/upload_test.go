package api

import "testing"

func TestOSSPartPlan(t *testing.T) {
	cases := []struct {
		name     string
		size     int64
		wantSize int64
		wantN    int
	}{
		{"tiny", 1, ossPartSize, 1},
		{"exactly one part", ossPartSize, ossPartSize, 1},
		{"just over one part", ossPartSize + 1, ossPartSize, 2},
		{"max parts boundary", ossPartSize * ossMaxParts, ossPartSize, ossMaxParts},
		{"grows past max parts", ossPartSize*ossMaxParts + 1, 0, ossMaxParts},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			partSize, parts := ossPartPlan(c.size)
			if parts != c.wantN {
				t.Fatalf("parts = %d, want %d", parts, c.wantN)
			}
			if c.wantSize != 0 && partSize != c.wantSize {
				t.Fatalf("partSize = %d, want %d", partSize, c.wantSize)
			}
			// The plan must always cover the whole file without an extra part.
			if int64(parts)*partSize < c.size {
				t.Fatalf("plan covers %d of %d bytes", int64(parts)*partSize, c.size)
			}
			if parts > 1 && int64(parts-1)*partSize >= c.size {
				t.Fatalf("last part is empty: %d parts of %d for %d bytes", parts, partSize, c.size)
			}
		})
	}
}
