package chat

import (
	"errors"
	"strings"
	"testing"
)

func mustRoot(t *testing.T, s *Store) Channel {
	t.Helper()
	root, err := s.EnsureRoot(testProject)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCreateMessage_IdempotentOnClientMsgID(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)

	m1, created, err := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-1", "hello", 0)
	if err != nil || !created {
		t.Fatalf("first create: created=%v err=%v", created, err)
	}
	m2, created, err := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-1", "different body", 0)
	if err != nil {
		t.Fatalf("duplicate create: %v", err)
	}
	if created {
		t.Fatal("duplicate clientMsgId reported created=true")
	}
	if m2.Seq != m1.Seq || m2.Body != "hello" {
		t.Fatalf("duplicate returned %+v, want the original message", m2)
	}
	if msgs, _ := s.ListMessages(testProject, root.ID, 0, 0); len(msgs) != 1 {
		t.Fatalf("duplicate created a second message: %d", len(msgs))
	}
}

func TestCreateMessage_ThreadInvariants(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)
	top, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-top", "top", 0)
	if err != nil {
		t.Fatal(err)
	}
	reply, _, err := s.CreateMessage(testProject, root.ID, "u2", "U2", "cm-reply", "reply", top.Seq)
	if err != nil {
		t.Fatalf("valid reply: %v", err)
	}

	cases := []struct {
		name       string
		threadRoot int64
	}{
		{"reply to a reply", reply.Seq},
		{"nonexistent root", 9999},
	}
	for _, c := range cases {
		if _, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-"+c.name, "x", c.threadRoot); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", c.name, err)
		}
	}

	// Reply to a deleted root.
	if _, err := s.DeleteMessage(testProject, root.ID, top.Seq); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-late", "x", top.Seq); !errors.Is(err, ErrInvalid) {
		t.Errorf("reply to deleted root: want ErrInvalid, got %v", err)
	}

	// Cross-channel root: the reply must target a root in the same channel.
	other, err := s.CreateChannel(testProject, "design-reviews", "", "u1", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherTop, _, err := s.CreateMessage(testProject, other.ID, "u1", "U1", "cm-other", "other channel", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-cross", "x", otherTop.Seq); !errors.Is(err, ErrInvalid) {
		t.Errorf("cross-channel thread root: want ErrInvalid, got %v", err)
	}
}

func TestCreateMessage_Validation(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)
	cases := []struct {
		name, clientID, body string
	}{
		{"empty body", "cm-e", "   "},
		{"oversize body", "cm-o", strings.Repeat("x", MaxBodyRunes+1)},
		{"missing clientMsgId", "", "hello"},
	}
	for _, c := range cases {
		if _, _, err := s.CreateMessage(testProject, root.ID, "u1", "U1", c.clientID, c.body, 0); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", c.name, err)
		}
	}
}

func TestEditAndDelete(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)
	m, _, _ := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-1", "original", 0)

	edited, err := s.EditMessage(testProject, root.ID, m.Seq, "revised")
	if err != nil || edited.Body != "revised" || edited.EditedAt == nil {
		t.Fatalf("edit: %+v err=%v", edited, err)
	}

	deleted, err := s.DeleteMessage(testProject, root.ID, m.Seq)
	if err != nil || deleted.DeletedAt == nil || deleted.Body != "" {
		t.Fatalf("delete: %+v err=%v", deleted, err)
	}
	// Idempotent delete; edit after delete refused.
	if _, err := s.DeleteMessage(testProject, root.ID, m.Seq); err != nil {
		t.Fatalf("re-delete: %v", err)
	}
	if _, err := s.EditMessage(testProject, root.ID, m.Seq, "zombie"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("edit after delete: want ErrInvalid, got %v", err)
	}
}

func TestReactions_ToggleIdempotent(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)
	m, _, _ := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-1", "hi", 0)

	r1, err := s.AddReaction(testProject, root.ID, m.Seq, "u2", "👍")
	if err != nil || len(r1.Reactions) != 1 {
		t.Fatalf("add: %+v err=%v", r1.Reactions, err)
	}
	r2, _ := s.AddReaction(testProject, root.ID, m.Seq, "u2", "👍")
	if len(r2.Reactions) != 1 {
		t.Fatalf("duplicate reaction stacked: %d", len(r2.Reactions))
	}
	r3, _ := s.RemoveReaction(testProject, root.ID, m.Seq, "u2", "👍")
	if len(r3.Reactions) != 0 {
		t.Fatalf("remove left %d", len(r3.Reactions))
	}
	if r4, _ := s.RemoveReaction(testProject, root.ID, m.Seq, "u2", "👍"); len(r4.Reactions) != 0 {
		t.Fatal("re-remove not idempotent")
	}
}

func TestListMessages_PagesBackward(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)
	seedMessages(t, s, 7)

	page1, err := s.ListMessages(testProject, root.ID, 0, 3)
	if err != nil || len(page1) != 3 {
		t.Fatalf("page1: %d err=%v", len(page1), err)
	}
	// Newest page, ascending within the page.
	if page1[0].Seq >= page1[2].Seq {
		t.Fatalf("page not ascending: %d..%d", page1[0].Seq, page1[2].Seq)
	}
	page2, _ := s.ListMessages(testProject, root.ID, page1[0].Seq, 3)
	if len(page2) != 3 || page2[2].Seq >= page1[0].Seq {
		t.Fatalf("page2 wrong window: %+v", seqs(page2))
	}
	page3, _ := s.ListMessages(testProject, root.ID, page2[0].Seq, 3)
	if len(page3) != 1 {
		t.Fatalf("page3: want the 1 remaining, got %d", len(page3))
	}
}

func TestListMessagesAfter_IncludesReplies(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)
	top, _, _ := s.CreateMessage(testProject, root.ID, "u1", "U1", "cm-1", "top", 0)
	cursor, err := s.LatestSeq(testProject, root.ID)
	if err != nil || cursor != top.Seq {
		t.Fatalf("LatestSeq = %d err=%v, want %d", cursor, err, top.Seq)
	}
	s.CreateMessage(testProject, root.ID, "u2", "U2", "cm-2", "reply", top.Seq)
	s.CreateMessage(testProject, root.ID, "u2", "U2", "cm-3", "another top", 0)

	delta, err := s.ListMessagesAfter(testProject, root.ID, cursor)
	if err != nil || len(delta) != 2 {
		t.Fatalf("delta: %d err=%v, want 2 (reply + top-level)", len(delta), err)
	}
}

func TestArchivedChannelRefusesPosts(t *testing.T) {
	s := newTestStore(t)
	mustRoot(t, s)
	ch, _ := s.CreateChannel(testProject, "temp", "", "u1", false, nil)
	if _, err := s.ArchiveChannel(testProject, ch.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateMessage(testProject, ch.ID, "u1", "U1", "cm-1", "too late", 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("post to archived: want ErrInvalid, got %v", err)
	}
}

func TestChannelInvariants(t *testing.T) {
	s := newTestStore(t)
	root := mustRoot(t, s)

	if _, err := s.ArchiveChannel(testProject, root.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("archive root: want ErrInvalid, got %v", err)
	}
	newName := "renamed"
	if _, err := s.UpdateChannel(testProject, root.ID, &newName, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("rename root: want ErrInvalid, got %v", err)
	}
	topic := "root topic is fine"
	if c, err := s.UpdateChannel(testProject, root.ID, nil, &topic); err != nil || c.Topic != topic {
		t.Errorf("set root topic: %+v err=%v", c, err)
	}

	if _, err := s.CreateChannel(testProject, "General", "", "u1", false, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("case-insensitive name collision: want ErrInvalid, got %v", err)
	}

	priv, err := s.CreateChannel(testProject, "secret", "", "owner-1", true, []string{"m-1", "m-1", "owner-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(priv.Members) != 2 {
		t.Fatalf("private members deduped wrong: %+v", priv.Members)
	}
	if priv.Members[0].UserID != "owner-1" || priv.Members[0].Role != "owner" {
		t.Fatalf("creator not owner: %+v", priv.Members[0])
	}

	pub, _ := s.CreateChannel(testProject, "open", "", "u1", false, nil)
	if _, err := s.AddChannelMember(testProject, pub.ID, "u9", "u1"); !errors.Is(err, ErrInvalid) {
		t.Errorf("ACL on public channel: want ErrInvalid, got %v", err)
	}
}

func seqs(ms []Message) []int64 {
	out := make([]int64, len(ms))
	for i, m := range ms {
		out[i] = m.Seq
	}
	return out
}
