package chat

import (
	"errors"
	"os"
	"testing"
)

func TestReadCursor_RoundtripAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s := newStoreAt(t, dir)
	root := mustRoot(t, s)
	for _, cm := range []string{"cm-1", "cm-2", "cm-3"} {
		if _, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", cm, "m "+cm, 0); err != nil {
			t.Fatal(err)
		}
	}

	u, advanced, err := s.SetReadCursor(testProject, "u1", root.ID, 2)
	if err != nil || !advanced {
		t.Fatalf("SetReadCursor: advanced=%v err=%v", advanced, err)
	}
	if u.LastReadSeq != 2 || u.UnreadCount != 1 || u.LatestSeq != 3 {
		t.Fatalf("unread after cursor=2: %+v", u)
	}

	// A new Store over the same dir (restart analog) sees the cursor.
	s2 := newStoreAt(t, dir)
	unreads, err := s2.Unreads(testProject, "u1", []Channel{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(unreads) != 1 || unreads[0].LastReadSeq != 2 || unreads[0].UnreadCount != 1 {
		t.Fatalf("reloaded unreads = %+v", unreads)
	}
}

func TestReadCursor_MonotonicAndClamped(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)
	for _, cm := range []string{"cm-1", "cm-2"} {
		if _, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", cm, "m", 0); err != nil {
			t.Fatal(err)
		}
	}

	if _, advanced, _ := s.SetReadCursor(testProject, "u1", root.ID, 2); !advanced {
		t.Fatal("first advance not reported")
	}
	// Backward move is a no-op that reports the stored cursor.
	u, advanced, err := s.SetReadCursor(testProject, "u1", root.ID, 1)
	if err != nil || advanced {
		t.Fatalf("backward move: advanced=%v err=%v", advanced, err)
	}
	if u.LastReadSeq != 2 {
		t.Fatalf("backward move rewound cursor to %d", u.LastReadSeq)
	}
	// A cursor past the newest message clamps to it.
	u, advanced, err = s.SetReadCursor(testProject, "u2", root.ID, 999)
	if err != nil || !advanced {
		t.Fatalf("clamped advance: advanced=%v err=%v", advanced, err)
	}
	if u.LastReadSeq != 2 || u.UnreadCount != 0 {
		t.Fatalf("clamped cursor = %+v, want lastReadSeq=2", u)
	}
}

func TestReadCursor_Validation(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)

	if _, _, err := s.SetReadCursor(testProject, "", root.ID, 1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty user key: %v", err)
	}
	if _, _, err := s.SetReadCursor(testProject, "u1", root.ID, -1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative seq: %v", err)
	}
	if _, _, err := s.SetReadCursor(testProject, "u1", "c999", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown channel: %v", err)
	}
}

func TestUnreads_CountingTable(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)

	// seq 1 top-level, seq 2 its reply, seq 3 top-level (deleted below),
	// seq 4 top-level.
	top, _, _ := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-1", "top", 0)
	if _, _, err := s.CreateMessage(testProject, root.ID, "u2", "U2", "cm-2", "reply", top.Seq); err != nil {
		t.Fatal(err)
	}
	doomed, _, _ := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-3", "doomed", 0)
	if _, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-4", "last", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteMessage(testProject, root.ID, doomed.Seq); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		cursor int64
		want   int
	}{
		{"never read: replies count, tombstones don't", 0, 3},
		{"mid-stream", 2, 1},
		{"cursor on the tombstone", 3, 1},
		{"fully read", 4, 0},
	}
	for _, c := range cases {
		if c.cursor > 0 {
			if _, _, err := s.SetReadCursor(testProject, "u-table", root.ID, c.cursor); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
		}
		unreads, err := s.Unreads(testProject, "u-table", []Channel{root})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(unreads) != 1 || unreads[0].UnreadCount != c.want {
			t.Errorf("%s: unreads = %+v, want count %d", c.name, unreads, c.want)
		}
	}
}

func TestUnreads_SkipsArchivedChannels(t *testing.T) {
	s := newTestStore(t)
	mustRoot(t, s)
	ch, err := s.CreateChannel(testProject, "old-news", "", "u1", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateMessage(testProject, ch.ID, "u1", "U1", "cm-1", "bye", 0); err != nil {
		t.Fatal(err)
	}
	archived, err := s.ArchiveChannel(testProject, ch.ID)
	if err != nil {
		t.Fatal(err)
	}
	unreads, err := s.Unreads(testProject, "u2", []Channel{archived})
	if err != nil {
		t.Fatal(err)
	}
	if len(unreads) != 0 {
		t.Fatalf("archived channel reported unreads: %+v", unreads)
	}
}

func TestCursors_CorruptFileBackedUp(t *testing.T) {
	dir := t.TempDir()
	s := newStoreAt(t, dir)
	root := mustRoot(t, s)
	if _, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-1", "m", 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SetReadCursor(testProject, "u1", root.ID, 1); err != nil {
		t.Fatal(err)
	}

	s2 := newStoreAt(t, dir)
	path := s2.cursorsPath(testProject)
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	// Corrupt cursors start fresh (positions lost, chat unaffected)…
	unreads, err := s2.Unreads(testProject, "u1", []Channel{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(unreads) != 1 || unreads[0].LastReadSeq != 0 {
		t.Fatalf("corrupt cursors not reset: %+v", unreads)
	}
	// …and the original is preserved as .bak.
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("no .bak after corrupt cursors.json: %v", err)
	}
}

func TestCursors_FutureVersionRefused(t *testing.T) {
	dir := t.TempDir()
	s := newStoreAt(t, dir)
	root := mustRoot(t, s)
	if _, _, err := s.SetReadCursor(testProject, "u1", root.ID, 0); err != nil {
		t.Fatal(err) // creates the project dir; seq 0 is a no-op advance
	}

	s2 := newStoreAt(t, dir)
	if err := os.WriteFile(s2.cursorsPath(testProject), []byte(`{"version":99,"cursors":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s2.SetReadCursor(testProject, "u1", root.ID, 1); !errors.Is(err, ErrFutureVersion) {
		t.Fatalf("future cursors version: %v", err)
	}
}
