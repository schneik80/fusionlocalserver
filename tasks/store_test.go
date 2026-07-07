package tasks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	projA = "urn:adsk.wipprod:fs.folder:co.projA"
	projB = "urn:adsk.wipprod:fs.folder:co.projB"
)

var alice = UserRef{ID: "sub-alice", Name: "Alice", Email: "alice@example.com"}
var bob = UserRef{ID: "sub-bob", Name: "Bob", Email: "bob@example.com"}

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s, dir
}

func TestCreateGetList(t *testing.T) {
	s, _ := newTestStore(t)
	created, err := s.Create(projA, "hub1", "Project A", Draft{Title: "  First task  "}, alice)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != "t1" || created.Num != 1 {
		t.Errorf("id/num = %q/%d, want t1/1", created.ID, created.Num)
	}
	if created.Title != "First task" {
		t.Errorf("title not trimmed: %q", created.Title)
	}
	if created.Status != "todo" || created.Priority != "medium" {
		t.Errorf("defaults = %q/%q, want todo/medium", created.Status, created.Priority)
	}
	if created.Rank != 1024 {
		t.Errorf("rank = %v, want 1024", created.Rank)
	}
	if created.DocRefs == nil {
		t.Error("DocRefs is nil, want empty slice")
	}
	got, err := s.Get(projA, "t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "First task" || got.CreatedBy.ID != alice.ID {
		t.Errorf("Get mismatch: %+v", got)
	}
	second, err := s.Create(projA, "hub1", "Project A", Draft{Title: "Second"}, alice)
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if second.ID != "t2" || second.Rank != 2048 {
		t.Errorf("second id/rank = %q/%v, want t2/2048", second.ID, second.Rank)
	}
	list, err := s.List(projA)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
}

func TestCreateValidation(t *testing.T) {
	s, _ := newTestStore(t)
	cases := []Draft{
		{Title: ""},
		{Title: "   "},
		{Title: strings.Repeat("x", MaxTitleRunes+1)},
		{Title: "ok", Description: strings.Repeat("d", MaxDescRunes+1)},
		{Title: "ok", Status: "bogus"},
		{Title: "ok", Priority: "bogus"},
		{Title: "ok", DueDate: "tomorrow"},
		{Title: "ok", DueDate: "2026-13-40"},
		{Title: "ok", DocRefs: []string{"https://example.com"}},
		{Title: "ok", DocRefs: make([]string, MaxDocRefs+1)},
	}
	for i, d := range cases {
		if len(d.DocRefs) > MaxDocRefs {
			for j := range d.DocRefs {
				d.DocRefs[j] = "fls:doc?hubId=h&itemId=i"
			}
		}
		if _, err := s.Create(projA, "hub1", "A", d, alice); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d: err = %v, want ErrInvalid", i, err)
		}
	}
}

func TestUpdatePatchAndClear(t *testing.T) {
	s, _ := newTestStore(t)
	created, _ := s.Create(projA, "hub1", "A", Draft{
		Title: "Task", DueDate: "2026-07-10", Assignee: &bob,
		DocRefs: []string{"fls:doc?hubId=h&itemId=i1"},
	}, alice)

	newTitle := "Renamed"
	newPrio := "high"
	got, err := s.Update(projA, created.ID, Patch{Title: &newTitle, Priority: &newPrio})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Title != "Renamed" || got.Priority != "high" {
		t.Errorf("patch not applied: %+v", got)
	}
	if got.DueDate != "2026-07-10" || got.Assignee == nil || got.Assignee.ID != bob.ID {
		t.Errorf("untouched fields changed: %+v", got)
	}
	if !got.UpdatedAt.After(created.UpdatedAt) && got.UpdatedAt.Equal(created.UpdatedAt) {
		t.Log("updatedAt unchanged (fast clock) — tolerated")
	}

	got, err = s.Update(projA, created.ID, Patch{ClearAssignee: true, ClearDueDate: true})
	if err != nil {
		t.Fatalf("Update clear: %v", err)
	}
	if got.Assignee != nil || got.DueDate != "" {
		t.Errorf("clear failed: %+v", got)
	}

	refs := []string{"fls:doc?hubId=h&itemId=i2", "fls:doc?hubId=h&itemId=i3"}
	got, err = s.Update(projA, created.ID, Patch{DocRefs: &refs})
	if err != nil {
		t.Fatalf("Update docRefs: %v", err)
	}
	if len(got.DocRefs) != 2 {
		t.Errorf("docRefs = %v", got.DocRefs)
	}

	bad := "bogus"
	if _, err := s.Update(projA, created.ID, Patch{Status: &bad}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad status err = %v, want ErrInvalid", err)
	}
	if _, err := s.Update(projA, "t999", Patch{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing task err = %v, want ErrNotFound", err)
	}
}

func TestStatusChangeAppendsToColumn(t *testing.T) {
	s, _ := newTestStore(t)
	t1, _ := s.Create(projA, "h", "A", Draft{Title: "one", Status: "done"}, alice) // done rank 1024
	t2, _ := s.Create(projA, "h", "A", Draft{Title: "two"}, alice)                 // todo rank 1024
	_ = t1
	done := "done"
	got, err := s.Update(projA, t2.ID, Patch{Status: &done})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Rank <= 1024 {
		t.Errorf("rank = %v, want > 1024 (appended after existing done task)", got.Rank)
	}
	// Explicit rank wins.
	rank := 512.0
	todo := "todo"
	got, err = s.Update(projA, t2.ID, Patch{Status: &todo, Rank: &rank})
	if err != nil {
		t.Fatalf("Update explicit rank: %v", err)
	}
	if got.Rank != 512 {
		t.Errorf("rank = %v, want 512", got.Rank)
	}
}

func TestDelete(t *testing.T) {
	s, _ := newTestStore(t)
	created, _ := s.Create(projA, "h", "A", Draft{Title: "doomed"}, alice)
	if err := s.Delete(projA, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(projA, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
	if err := s.Delete(projA, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("double delete = %v, want ErrNotFound", err)
	}
}

func TestMine(t *testing.T) {
	s, _ := newTestStore(t)
	_, _ = s.Create(projA, "hub1", "Project A", Draft{Title: "mine by creation"}, alice)
	_, _ = s.Create(projA, "hub1", "Project A", Draft{Title: "assigned to alice", Assignee: &alice}, bob)
	_, _ = s.Create(projB, "hub1", "Project B", Draft{Title: "unrelated"}, bob)
	// Email-only match (session predating the sub claim).
	_, _ = s.Create(projB, "hub1", "Project B", Draft{
		Title:    "assigned by email",
		Assignee: &UserRef{ID: "sub-other", Email: "ALICE@example.com"},
	}, bob)

	mine, err := s.Mine(alice.ID, alice.Email)
	if err != nil {
		t.Fatalf("Mine: %v", err)
	}
	if len(mine) != 3 {
		t.Fatalf("Mine len = %d, want 3: %+v", len(mine), mine)
	}
	for _, pt := range mine {
		if pt.ProjectID == "" || pt.HubID == "" || pt.ProjectName == "" {
			t.Errorf("missing project annotation: %+v", pt)
		}
	}
	bobs, err := s.Mine(bob.ID, bob.Email)
	if err != nil {
		t.Fatalf("Mine bob: %v", err)
	}
	if len(bobs) != 3 {
		t.Errorf("Mine bob len = %d, want 3 (creator of 3)", len(bobs))
	}
}

func TestPersistenceAcrossReload(t *testing.T) {
	s, dir := newTestStore(t)
	created, _ := s.Create(projA, "hub1", "Project A", Draft{Title: "persisted", Assignee: &bob}, alice)

	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore reload: %v", err)
	}
	got, err := s2.Get(projA, created.ID)
	if err != nil {
		t.Fatalf("Get after reload: %v", err)
	}
	if got.Title != "persisted" || got.Assignee == nil || got.Assignee.ID != bob.ID {
		t.Errorf("reload mismatch: %+v", got)
	}
	// Counter survives: next create continues the sequence.
	next, err := s2.Create(projA, "hub1", "Project A", Draft{Title: "next"}, alice)
	if err != nil {
		t.Fatalf("Create after reload: %v", err)
	}
	if next.ID != "t2" {
		t.Errorf("next id = %q, want t2", next.ID)
	}
}

func TestCorruptFileRecovers(t *testing.T) {
	s, dir := newTestStore(t)
	_, _ = s.Create(projA, "h", "A", Draft{Title: "x"}, alice)
	path := filepath.Join(dir, sanitizeID(projA), "tasks.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	s2, _ := NewStore(dir)
	list, err := s2.List(projA)
	if err != nil {
		t.Fatalf("List after corruption: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List = %v, want empty fresh state", list)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("no .bak preserved: %v", err)
	}
}

func TestFutureVersionRefused(t *testing.T) {
	s, dir := newTestStore(t)
	_, _ = s.Create(projA, "h", "A", Draft{Title: "x"}, alice)
	path := filepath.Join(dir, sanitizeID(projA), "tasks.json")
	if err := os.WriteFile(path, []byte(`{"version": 99, "projectId": "p", "nextTaskId": 1, "tasks": []}`), 0600); err != nil {
		t.Fatal(err)
	}
	s2, _ := NewStore(dir)
	if _, err := s2.List(projA); !errors.Is(err, ErrFutureVersion) {
		t.Errorf("List = %v, want ErrFutureVersion", err)
	}
	// Mine skips the bad project instead of failing.
	mine, err := s2.Mine(alice.ID, alice.Email)
	if err != nil {
		t.Fatalf("Mine: %v", err)
	}
	if len(mine) != 0 {
		t.Errorf("Mine = %v, want empty", mine)
	}
}
