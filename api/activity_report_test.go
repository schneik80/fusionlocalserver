package api

import (
	"testing"
	"time"
)

func ts(y int, mo time.Month, d, h int) time.Time {
	return time.Date(y, mo, d, h, 0, 0, 0, time.UTC)
}

// sampleEvents: 4 events, 3 designs, 2 projects, 2 contributors, across 2 months.
func sampleEvents() []ActivityEvent {
	return []ActivityEvent{
		{EntityType: "design", EntityID: "A1", LineageURN: "la", EntityName: "Alpha", HubID: "imallc", HubName: "IMA LLC",
			ProjectID: "P1", ProjectName: "Proj One", FolderURN: "urn:x:fs.folder:co.F1",
			VersionNumber: 1, Action: ActionCreated, Timestamp: ts(2026, 1, 10, 10), CreatedOn: ts(2026, 1, 10, 9),
			Actor: Actor{AccountID: "u-alice", DisplayName: "Alice"}},
		{EntityType: "design", EntityID: "A1", LineageURN: "la", EntityName: "Alpha", HubID: "imallc", HubName: "IMA LLC",
			ProjectID: "P1", ProjectName: "Proj One", FolderURN: "urn:x:fs.folder:co.F1",
			VersionNumber: 2, Action: ActionUpdated, Timestamp: ts(2026, 1, 10, 15), CreatedOn: ts(2026, 1, 10, 9),
			Actor: Actor{AccountID: "u-bob", DisplayName: "Bob"}},
		{EntityType: "design", EntityID: "A2", LineageURN: "lb", EntityName: "Beta", HubID: "imallc", HubName: "IMA LLC",
			ProjectID: "P1", ProjectName: "Proj One", FolderURN: "urn:x:fs.folder:co.F1",
			VersionNumber: 3, Action: ActionUpdated, Timestamp: ts(2026, 2, 5, 12), CreatedOn: ts(2025, 12, 1, 0),
			Actor: Actor{AccountID: "u-alice", DisplayName: "Alice"}},
		{EntityType: "design", EntityID: "B1", LineageURN: "lc", EntityName: "Gamma", HubID: "imallc", HubName: "IMA LLC",
			ProjectID: "P2", ProjectName: "Proj Two", FolderURN: "urn:x:fs.folder:co.F2",
			VersionNumber: 1, Action: ActionCreated, Timestamp: ts(2026, 2, 20, 8), CreatedOn: ts(2026, 2, 20, 8),
			Actor: Actor{AccountID: "u-bob", DisplayName: "Bob"}},
	}
}

func TestBuildReport_HubScope(t *testing.T) {
	rep := BuildReport(sampleEvents(), ScopeHub, "imallc", BucketDay, time.Time{}, time.Time{})

	if rep.TotalEvents != 4 {
		t.Errorf("TotalEvents = %d, want 4", rep.TotalEvents)
	}
	if rep.DesignCount != 3 {
		t.Errorf("DesignCount = %d, want 3", rep.DesignCount)
	}
	if rep.VersionCount != 6 { // la:2 + lb:3 + lc:1
		t.Errorf("VersionCount = %d, want 6", rep.VersionCount)
	}
	if rep.ContributorCount != 2 {
		t.Errorf("ContributorCount = %d, want 2", rep.ContributorCount)
	}
	if rep.ScopeName != "IMA LLC" {
		t.Errorf("ScopeName = %q, want IMA LLC", rep.ScopeName)
	}
	if want := ts(2025, 12, 1, 0); !rep.CreatedOn.Equal(want) {
		t.Errorf("CreatedOn = %s, want %s", rep.CreatedOn, want)
	}
	if want := ts(2026, 2, 20, 8); !rep.LastChange.Equal(want) {
		t.Errorf("LastChange = %s, want %s", rep.LastChange, want)
	}
	// Day timeline: 2026-01-10 (2), 2026-02-05 (1), 2026-02-20 (1).
	if len(rep.Timeline) != 3 {
		t.Fatalf("day timeline buckets = %d, want 3", len(rep.Timeline))
	}
	if rep.Timeline[0].Count != 2 || !rep.Timeline[0].Start.Equal(ts(2026, 1, 10, 0)) {
		t.Errorf("first day bucket = %+v, want {2026-01-10 count 2}", rep.Timeline[0])
	}
	// Children = projects, P1 (3) before P2 (1).
	if len(rep.Children) != 2 || rep.Children[0].Type != "project" {
		t.Fatalf("children = %+v, want 2 projects", rep.Children)
	}
	if rep.Children[0].ID != "P1" || rep.Children[0].EventCount != 3 {
		t.Errorf("top child = %+v, want P1 count 3", rep.Children[0])
	}
	// Contributors tie at 2 each → Alice before Bob.
	if rep.Contributors[0].DisplayName != "Alice" || rep.Contributors[0].EventCount != 2 {
		t.Errorf("top contributor = %+v, want Alice/2", rep.Contributors[0])
	}
}

func TestBuildReport_MonthBucket(t *testing.T) {
	rep := BuildReport(sampleEvents(), ScopeHub, "imallc", BucketMonth, time.Time{}, time.Time{})
	if len(rep.Timeline) != 2 {
		t.Fatalf("month timeline buckets = %d, want 2", len(rep.Timeline))
	}
	if !rep.Timeline[0].Start.Equal(ts(2026, 1, 1, 0)) || rep.Timeline[0].Count != 2 {
		t.Errorf("jan bucket = %+v, want {2026-01 count 2}", rep.Timeline[0])
	}
	if !rep.Timeline[1].Start.Equal(ts(2026, 2, 1, 0)) || rep.Timeline[1].Count != 2 {
		t.Errorf("feb bucket = %+v, want {2026-02 count 2}", rep.Timeline[1])
	}
}

func TestBuildReport_ProjectScope(t *testing.T) {
	rep := BuildReport(sampleEvents(), ScopeProject, "P1", BucketDay, time.Time{}, time.Time{})
	if rep.TotalEvents != 3 {
		t.Errorf("P1 TotalEvents = %d, want 3", rep.TotalEvents)
	}
	if rep.ScopeName != "Proj One" {
		t.Errorf("P1 ScopeName = %q, want Proj One", rep.ScopeName)
	}
	if rep.DesignCount != 2 || rep.VersionCount != 5 { // la:2 + lb:3
		t.Errorf("P1 designs/versions = %d/%d, want 2/5", rep.DesignCount, rep.VersionCount)
	}
	if len(rep.Children) != 1 || rep.Children[0].Type != "folder" {
		t.Errorf("P1 children = %+v, want 1 folder", rep.Children)
	}
}

func TestBuildReport_DesignScope(t *testing.T) {
	rep := BuildReport(sampleEvents(), ScopeDesign, "la", BucketDay, time.Time{}, time.Time{})
	if rep.TotalEvents != 2 {
		t.Errorf("design TotalEvents = %d, want 2", rep.TotalEvents)
	}
	if rep.DesignCount != 1 || rep.VersionCount != 2 {
		t.Errorf("design designs/versions = %d/%d, want 1/2", rep.DesignCount, rep.VersionCount)
	}
	if rep.ScopeName != "Alpha" {
		t.Errorf("design ScopeName = %q, want Alpha", rep.ScopeName)
	}
	if len(rep.Children) != 0 {
		t.Errorf("design children = %+v, want none", rep.Children)
	}
	// Events newest-first.
	if len(rep.Events) != 2 || rep.Events[0].VersionNumber != 2 {
		t.Errorf("design events not newest-first: %+v", rep.Events)
	}
}

func TestBuildReport_TimeWindow(t *testing.T) {
	// Only February events.
	from := ts(2026, 2, 1, 0)
	rep := BuildReport(sampleEvents(), ScopeHub, "imallc", BucketDay, from, time.Time{})
	if rep.TotalEvents != 2 {
		t.Errorf("windowed TotalEvents = %d, want 2", rep.TotalEvents)
	}
}
