package chat

import (
	"fmt"
	"sync"
	"testing"
)

// TestConcurrentAppendSafety hammers one channel from many goroutines and
// asserts the store hands out unique, contiguous seqs and that the JSONL
// log replays to exactly the same set. Run with -race where a C toolchain
// is available.
func TestConcurrentAppendSafety(t *testing.T) {
	dir := t.TempDir()
	s := newStoreAt(t, dir)
	root := mustRoot(t, s)

	const workers = 20
	const perWorker = 50
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				id := fmt.Sprintf("cm-%d-%d", w, i)
				if _, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", id, "body "+id, 0); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	const total = workers * perWorker
	check := func(s *Store, label string) {
		t.Helper()
		msgs, err := s.ListMessages(testProject, root.ID, 0, 200)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		// Page through everything.
		all := append([]Message{}, msgs...)
		for len(all) < total {
			page, err := s.ListMessages(testProject, root.ID, all[0].Seq, 200)
			if err != nil || len(page) == 0 {
				t.Fatalf("%s: paging stalled at %d (%v)", label, len(all), err)
			}
			all = append(page, all...)
		}
		if len(all) != total {
			t.Fatalf("%s: %d messages, want %d", label, len(all), total)
		}
		seen := make(map[int64]bool, total)
		for _, m := range all {
			if seen[m.Seq] {
				t.Fatalf("%s: duplicate seq %d", label, m.Seq)
			}
			seen[m.Seq] = true
		}
		for seq := int64(1); seq <= int64(total); seq++ {
			if !seen[seq] {
				t.Fatalf("%s: seq %d missing (not contiguous)", label, seq)
			}
		}
	}
	check(s, "live")
	s.Close()

	s2 := newStoreAt(t, dir)
	check(s2, "replayed")
}
