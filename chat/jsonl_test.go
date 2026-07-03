package chat

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedMessages posts n top-level messages and returns the store dir.
func seedMessages(t *testing.T, s *Store, n int) {
	t.Helper()
	root, err := s.EnsureRoot(testProject)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if _, _, err := s.CreateMessage(testProject, root.ID, "u1", "User One", fmt.Sprintf("cm-%d", i), fmt.Sprintf("message %d", i), 0); err != nil {
			t.Fatalf("CreateMessage %d: %v", i, err)
		}
	}
}

func TestReplay_CrashTailTrimmedAndAppendable(t *testing.T) {
	dir := t.TempDir()
	s := newStoreAt(t, dir)
	seedMessages(t, s, 5)
	root, _ := s.EnsureRoot(testProject)
	logPath := s.logPath(testProject, root.ID)
	s.Close()

	// Simulate a crash mid-append: chop the file mid-way through the last line.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, data[:len(data)-10], 0600); err != nil {
		t.Fatal(err)
	}

	s2 := newStoreAt(t, dir)
	msgs, err := s2.ListMessages(testProject, root.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages after trim: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("want 4 messages after losing the crash tail, got %d", len(msgs))
	}

	// The next append must land on a clean line and replay afterwards.
	if _, _, err := s2.CreateMessage(testProject, root.ID, "u1", "User One", "cm-after", "post-crash", 0); err != nil {
		t.Fatalf("CreateMessage after trim: %v", err)
	}
	if got, _ := s2.ListMessages(testProject, root.ID, 0, 0); len(got) != 5 {
		t.Fatalf("want 5 after post-crash append, got %d", len(got))
	}
	s2.Close()

	s3 := newStoreAt(t, dir)
	got, err := s3.ListMessages(testProject, root.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got[4].Body != "post-crash" {
		t.Fatalf("replay after post-crash append: got %d messages", len(got))
	}
	// Seq of the post-crash message must not collide with the trimmed one.
	seen := map[int64]bool{}
	for _, m := range got {
		if seen[m.Seq] {
			t.Fatalf("duplicate seq %d after crash recovery", m.Seq)
		}
		seen[m.Seq] = true
	}
}

func TestReplay_MidFileCorruptionBacksUp(t *testing.T) {
	dir := t.TempDir()
	s := newStoreAt(t, dir)
	seedMessages(t, s, 3)
	root, _ := s.EnsureRoot(testProject)
	logPath := s.logPath(testProject, root.ID)
	s.Close()

	data, _ := os.ReadFile(logPath)
	lines := strings.SplitAfter(string(data), "\n")
	lines[1] = "{garbage}\n"
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "")), 0600); err != nil {
		t.Fatal(err)
	}

	s2 := newStoreAt(t, dir)
	msgs, err := s2.ListMessages(testProject, root.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListMessages over corrupt log: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want the valid prefix (1 message), got %d", len(msgs))
	}
	if _, err := os.Stat(logPath + ".bak"); err != nil {
		t.Fatalf("corrupt log not preserved as .bak: %v", err)
	}
}

func TestReplay_FutureRecordVersionRefused(t *testing.T) {
	dir := t.TempDir()
	s := newStoreAt(t, dir)
	root, err := s.EnsureRoot(testProject)
	if err != nil {
		t.Fatal(err)
	}
	logPath := s.logPath(testProject, root.ID)
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"v":%d,"op":"create","seq":1,"authorId":"u","clientMsgId":"x","body":"b","at":"2026-01-01T00:00:00Z"}`+"\n", recordVersion+1)
	if err := os.WriteFile(logPath, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ListMessages(testProject, root.ID, 0, 0); !errors.Is(err, ErrFutureVersion) {
		t.Fatalf("want ErrFutureVersion, got %v", err)
	}
}

func TestReplay_DerivedThreadCounters(t *testing.T) {
	dir := t.TempDir()
	s := newStoreAt(t, dir)
	root, _ := s.EnsureRoot(testProject)
	rootMsg, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-root", "root message", 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, _, err := s.CreateMessage(testProject, root.ID, "u2", "U2", fmt.Sprintf("cm-r%d", i), "reply", rootMsg.Seq); err != nil {
			t.Fatal(err)
		}
	}
	// Delete one reply: live-count drops to 2.
	thread, _ := s.ListThread(testProject, root.ID, rootMsg.Seq)
	if _, err := s.DeleteMessage(testProject, root.ID, thread[1].Seq); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Counters must be identical after a full replay.
	s2 := newStoreAt(t, dir)
	msgs, err := s2.ListMessages(testProject, root.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got *Message
	for i := range msgs {
		if msgs[i].Seq == rootMsg.Seq {
			got = &msgs[i]
		}
	}
	if got == nil {
		t.Fatal("root message missing after replay")
	}
	if got.ReplyCount != 2 {
		t.Fatalf("replayed ReplyCount = %d, want 2 (3 replies, 1 deleted)", got.ReplyCount)
	}
	if got.LastReplyAt == nil {
		t.Fatal("replayed LastReplyAt missing")
	}
}
