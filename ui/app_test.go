package ui

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/schneik80/FusionDataCLI/api"
	"github.com/schneik80/FusionDataCLI/auth"
	"github.com/schneik80/FusionDataCLI/fusion"
	"github.com/schneik80/FusionDataCLI/internal/testutil"
)

// TestUpdate_TokenReadyMsg_TransitionsToLoading drives the model from the
// pre-auth waiting state into the hub-loading state by feeding it a
// successful tokenReadyMsg. It locks down the contract that a non-empty
// token transitions to stateLoading + hubLoading and dispatches a hub-load
// command.
func TestUpdate_TokenReadyMsg_TransitionsToLoading(t *testing.T) {
	m := Model{
		state:        stateLoading,
		spinner:      spinner.New(),
		styleCache:   &styleCache{},
		detailsCache: map[string]*api.ItemDetails{},
	}
	updated, cmd := m.Update(tokenReadyMsg{token: "abc"})
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update did not return Model, got %T", updated)
	}
	if um.token != "abc" {
		t.Errorf("token = %q, want %q", um.token, "abc")
	}
	if um.state != stateLoading {
		t.Errorf("state = %d, want stateLoading (%d)", um.state, stateLoading)
	}
	if !um.hubLoading {
		t.Errorf("hubLoading = false, want true")
	}
	if cmd == nil {
		t.Errorf("cmd = nil, want non-nil load-hubs cmd")
	}
}

// TestUpdate_TokenReadyMsg_EmptyTokenGoesAuthNeeded confirms the auth
// fall-through: when checkTokensCmd reports no token, the UI moves to
// stateAuthNeeded and emits no command.
func TestUpdate_TokenReadyMsg_EmptyTokenGoesAuthNeeded(t *testing.T) {
	m := Model{
		state:        stateLoading,
		spinner:      spinner.New(),
		styleCache:   &styleCache{},
		detailsCache: map[string]*api.ItemDetails{},
	}
	updated, cmd := m.Update(tokenReadyMsg{token: ""})
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update did not return Model, got %T", updated)
	}
	if um.state != stateAuthNeeded {
		t.Errorf("state = %d, want stateAuthNeeded (%d)", um.state, stateAuthNeeded)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
}

// TestUpdate_KeyQuit asserts that pressing q in the browsing state
// returns tea.Quit, which evaluates to a tea.QuitMsg{} when invoked.
func TestUpdate_KeyQuit(t *testing.T) {
	m := sampleBrowsingModel(120, 40)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("cmd = nil, want tea.Quit")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("cmd() returned %T, want tea.QuitMsg", msg)
	}
}

// TestUpdate_NavigateRight_LoadsContents is the marquee Phase 3 test. It
// drives a right-arrow press from stateBrowsing/colProjects through to
// the contentsLoadedMsg returned by loadProjectContentsCmd's fan-in,
// exercising the full chain: state transition → fan-out tea.Cmd →
// concurrent api.GetFolders + api.GetProjectItems → merge into a
// single contentsLoadedMsg.
func TestUpdate_NavigateRight_LoadsContents(t *testing.T) {
	handler := func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		if req.AuthHeader != "Bearer test-token" {
			t.Errorf("AuthHeader = %q, want %q", req.AuthHeader, "Bearer test-token")
		}
		if got := req.Variables["projectId"]; got != "proj-1" {
			t.Errorf("projectId = %v, want \"proj-1\"", got)
		}
		if strings.Contains(req.Query, "foldersByProject") {
			return testutil.GraphQLResponse{Data: map[string]any{
				"foldersByProject": map[string]any{
					"pagination": map[string]any{"cursor": ""},
					"results": []map[string]any{
						{"id": "folder-1", "name": "Drawings"},
					},
				},
			}}
		}
		return testutil.GraphQLResponse{Data: map[string]any{
			"itemsByProject": map[string]any{
				"pagination": map[string]any{"cursor": ""},
				"results": []map[string]any{
					{"__typename": "DesignItem", "id": "design-1", "name": "Widget"},
				},
			},
		}}
	}
	srv := testutil.GraphQLServer(t, handler)
	t.Cleanup(api.SetGraphqlEndpointForTesting(srv.URL))

	m := Model{
		state:         stateBrowsing,
		width:         120,
		height:        40,
		activeCol:     colProjects,
		token:         "test-token",
		selectedHubID: "hub-1",
		cols: [numCols][]api.NavItem{
			{{ID: "proj-1", Name: "Project A", Kind: "project", AltID: "alt-1", IsContainer: true}},
			nil,
		},
		cursors:      [numCols]int{0, 0},
		spinner:      spinner.New(),
		styleCache:   &styleCache{},
		detailsCache: map[string]*api.ItemDetails{},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update did not return Model, got %T", updated)
	}
	if um.activeCol != colContents {
		t.Errorf("activeCol = %d, want colContents (%d)", um.activeCol, colContents)
	}
	if !um.loading[colContents] {
		t.Errorf("loading[colContents] = false, want true")
	}
	if um.selectedProjectAltID != "alt-1" {
		t.Errorf("selectedProjectAltID = %q, want %q", um.selectedProjectAltID, "alt-1")
	}
	if cmd == nil {
		t.Fatalf("cmd = nil, want load-contents cmd")
	}

	msg := cmd()
	loaded, ok := msg.(contentsLoadedMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want contentsLoadedMsg", msg)
	}
	if got, want := len(loaded.items), 2; got != want {
		t.Fatalf("len(items) = %d, want %d (got items=%+v)", got, want, loaded.items)
	}
	// Folders come before items per loadProjectContentsCmd's append order.
	if loaded.items[0].Kind != "folder" || !loaded.items[0].IsContainer {
		t.Errorf("items[0] = %+v, want kind=folder & IsContainer=true", loaded.items[0])
	}
	if loaded.items[0].Name != "Drawings" {
		t.Errorf("items[0].Name = %q, want %q", loaded.items[0].Name, "Drawings")
	}
	if loaded.items[1].Kind != "design" {
		t.Errorf("items[1].Kind = %q, want \"design\"", loaded.items[1].Kind)
	}
	if loaded.items[1].Name != "Widget" {
		t.Errorf("items[1].Name = %q, want %q", loaded.items[1].Name, "Widget")
	}
}

// TestVerifySameHub covers the four code paths in verifySameHub:
//   - exact ID match (after NormalizeProjectID)
//   - case-insensitive name fallback when the ID can't be parsed
//   - no match → returns the "different hub" error naming the expected hub
//   - empty inputs short-circuit before the MCP call
func TestVerifySameHub(t *testing.T) {
	srv := testutil.NewMCPServer(t, testutil.MCPScenario{
		SessionID: "verify-sid",
		Tools: map[string]testutil.MCPHandler{
			"fusion_mcp_read": func(args map[string]any) testutil.MCPResponse {
				return testutil.MCPResponse{
					ContentText: `{"success": true, "projects": [` +
						`{"id": "20250213876602531", "name": "Buggy"},` +
						`{"id": "98765432101234567", "name": "Robot"}` +
						`]}`,
				}
			},
		},
	})
	client := &fusion.Client{Endpoint: srv.URL, HTTP: srv.Client()}

	encode := func(plain string) string {
		return "a." + base64.RawURLEncoding.EncodeToString([]byte(plain))
	}

	cases := []struct {
		name      string
		altID     string
		projName  string
		hubName   string
		wantErr   bool
		errSubstr []string
	}{
		{
			name:    "exact_id_match",
			altID:   encode("business:autodesk#20250213876602531"),
			hubName: "MyHub",
			wantErr: false,
		},
		{
			name:     "name_match_when_id_unparseable",
			altID:    "garbage",
			projName: "Robot",
			hubName:  "MyHub",
			wantErr:  false,
		},
		{
			name:      "no_match",
			altID:     encode("business:autodesk#9999"),
			projName:  "Nope",
			hubName:   "MyHub",
			wantErr:   true,
			errSubstr: []string{"different hub", "MyHub"},
		},
		{
			name:    "empty_skips_check",
			altID:   "",
			wantErr: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := verifySameHub(ctx, client, tc.altID, tc.projName, tc.hubName)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("verifySameHub: expected error, got nil")
				}
				for _, sub := range tc.errSubstr {
					if !strings.Contains(err.Error(), sub) {
						t.Errorf("verifySameHub: error %q missing substring %q", err.Error(), sub)
					}
				}
				return
			}
			if err != nil {
				t.Errorf("verifySameHub: unexpected error: %v", err)
			}
		})
	}
}

// TestRecoverFromError_AuthError_DeletesTokens locks down the auth-recovery
// behaviour: when the model is in stateError with an auth-flavored error,
// recoverFromError must remove the on-disk token file so the next
// checkTokensCmd run prompts for fresh login. Non-auth recovery paths skip
// the token deletion (covered implicitly by the explicit auth-error
// trigger here).
func TestRecoverFromError_AuthError_DeletesTokens(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	td := &auth.TokenData{
		AccessToken:  "x",
		RefreshToken: "y",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	if err := auth.SaveTokens(td); err != nil {
		t.Fatalf("SaveTokens: unexpected error: %v", err)
	}
	// Sanity-check that the file is on disk before we ask recoverFromError
	// to delete it — otherwise a missing-file false-positive would mask a
	// regression where the function silently does nothing.
	if got, err := auth.LoadTokens(); err != nil || got == nil {
		t.Fatalf("LoadTokens before recover: got=%v err=%v, want non-nil token", got, err)
	}

	m := Model{
		state:        stateError,
		err:          errors.New("401 unauthorized"),
		spinner:      spinner.New(),
		styleCache:   &styleCache{},
		detailsCache: map[string]*api.ItemDetails{},
	}
	if !isAuthError(m.err) {
		t.Fatalf("test setup: isAuthError(%v) = false; pick an auth-flavored message", m.err)
	}
	updated, cmd := m.recoverFromError()
	if updated.state != stateLoading {
		t.Errorf("state after recover = %d, want stateLoading (%d)", updated.state, stateLoading)
	}
	if updated.err != nil {
		t.Errorf("err after recover = %v, want nil", updated.err)
	}
	if cmd == nil {
		t.Errorf("cmd after recover = nil, want non-nil (spinner.Tick + checkTokensCmd batch)")
	}

	got, err := auth.LoadTokens()
	if err != nil {
		t.Fatalf("LoadTokens after recover: unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("LoadTokens after recover: got %+v, want nil (file should be deleted)", got)
	}
}

// TestBreadcrumb_HitDetection exercises the pure buildBreadcrumb helper
// against a model with hub + project + a 2-deep folder stack, verifying
// the rendered text, the number of clickable hits, their kinds, and
// folder-stack indices.
func TestBreadcrumb_HitDetection(t *testing.T) {
	m := Model{
		width:                120,
		height:               40,
		selectedHubNameCache: "Hub",
		cols: [numCols][]api.NavItem{
			{{ID: "p1", Name: "Project", Kind: "project"}},
			{{ID: "f2", Name: "Subfolder", Kind: "folder", IsContainer: true}},
		},
		cursors: [numCols]int{0, 0},
		folderStack: []breadcrumbEntry{
			{id: "f1", name: "Outer"},
			{id: "f2", name: "Inner"},
		},
		activeCol:    colContents,
		spinner:      spinner.New(),
		styleCache:   &styleCache{},
		detailsCache: map[string]*api.ItemDetails{},
	}

	text, hits := m.buildBreadcrumb(breadcrumbXOffset())

	for _, want := range []string{"Hub", "Project", "Outer", "Inner"} {
		if !strings.Contains(text, want) {
			t.Errorf("breadcrumb text %q missing segment %q", text, want)
		}
	}
	if !strings.Contains(text, " › ") {
		t.Errorf("breadcrumb text %q missing separator %q", text, " › ")
	}

	if len(hits) != 4 {
		t.Fatalf("len(hits) = %d, want 4 (hub + project + 2 folders); hits=%+v", len(hits), hits)
	}

	wantKinds := []string{"hub", "project", "folder", "folder"}
	off := breadcrumbXOffset()
	for i, h := range hits {
		if h.kind != wantKinds[i] {
			t.Errorf("hits[%d].kind = %q, want %q", i, h.kind, wantKinds[i])
		}
		if h.xStart < off {
			t.Errorf("hits[%d].xStart = %d, want >= %d", i, h.xStart, off)
		}
		if h.xEnd <= h.xStart {
			t.Errorf("hits[%d]: xEnd %d <= xStart %d", i, h.xEnd, h.xStart)
		}
	}

	if hits[2].index != 0 {
		t.Errorf("hits[2].index = %d, want 0 (Outer folder)", hits[2].index)
	}
	if hits[3].index != 1 {
		t.Errorf("hits[3].index = %d, want 1 (Inner folder)", hits[3].index)
	}
}

// designModelWithTabs returns a stateBrowsing Model whose currently
// selected item is a DesignItem with a populated RootComponentVersionID,
// so the four-tab strip is available. Used by tab-dispatch tests.
func designModelWithTabs() Model {
	sp := spinner.New()
	return Model{
		state:         stateBrowsing,
		width:         120,
		height:        40,
		spinner:       sp,
		version:       "test",
		token:         "tok",
		activeCol:     colContents,
		selectedHubID: "H1",
		cols: [numCols][]api.NavItem{
			{{ID: "P1", Name: "Project", Kind: "project", IsContainer: true}},
			{{ID: "D1", Name: "Design.f3d", Kind: "design"}},
		},
		details: &api.ItemDetails{
			ID:                     "D1",
			Name:                   "Design.f3d",
			Typename:               "DesignItem",
			RootComponentVersionID: "CV1",
		},
		detailsCache:   map[string]*api.ItemDetails{},
		usesCache:      map[string][]api.ComponentRef{},
		whereUsedCache: map[string][]api.ComponentRef{},
		drawingsCache:  map[string][]api.DrawingRef{},
		styleCache:     &styleCache{},
	}
}

// TestUpdate_TabSelect_DispatchesLoad verifies that pressing a number key
// 1-4 switches the active tab and, for non-Details tabs, returns a fetch
// command. Cache hits should produce a switch with no command.
func TestUpdate_TabSelect_DispatchesLoad(t *testing.T) {
	cases := []struct {
		key  rune
		want detailsTab
	}{
		{key: '2', want: tabUses},
		{key: '3', want: tabWhereUsed},
		{key: '4', want: tabDrawings},
		{key: '1', want: tabDetails},
	}
	for _, tc := range cases {
		t.Run(string(tc.key), func(t *testing.T) {
			m := designModelWithTabs()
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tc.key}})
			um := updated.(Model)
			if um.detailsTab != tc.want {
				t.Errorf("detailsTab = %d, want %d", um.detailsTab, tc.want)
			}
			if tc.want == tabDetails {
				if cmd != nil {
					t.Errorf("Details tab should not emit a command, got %T", cmd)
				}
				return
			}
			if !um.tabLoading[tc.want] {
				t.Errorf("tabLoading[%d] = false, want true (fetch in flight)", tc.want)
			}
			if cmd == nil {
				t.Errorf("expected non-nil load cmd for tab %d", tc.want)
			}
		})
	}
}

// TestUpdate_TabSelect_NoTabsAvailableIsNoop confirms tabs are inert
// for items that have no tab strip at all (BasicItem,
// ConfiguredDesignItem, etc.): pressing 1-4 changes nothing and emits
// no cmd. Drawings used to live here too, but they now have a Uses
// tab — see TestUpdate_TabSelect_DrawingItemUsesAvailable.
func TestUpdate_TabSelect_NoTabsAvailableIsNoop(t *testing.T) {
	m := designModelWithTabs()
	m.details.Typename = "BasicItem"
	m.details.RootComponentVersionID = ""

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	um := updated.(Model)
	if um.detailsTab != tabDetails {
		t.Errorf("detailsTab = %d, want tabDetails for item type with no tabs", um.detailsTab)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd, got %T", cmd)
	}
}

// TestUpdate_TabSelect_DrawingItemUsesAvailable confirms drawings
// expose Uses (the source design) but not Where Used or Drawings.
// Pressing 2 selects Uses and triggers a fetch; pressing 3 or 4 is a
// no-op because those tabs don't apply to drawings.
func TestUpdate_TabSelect_DrawingItemUsesAvailable(t *testing.T) {
	m := designModelWithTabs()
	m.details.Typename = "DrawingItem"
	m.details.RootComponentVersionID = ""
	m.details.ID = "di-drawing-1"

	// 2 → Uses (works on drawings)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	um := updated.(Model)
	if um.detailsTab != tabUses {
		t.Errorf("after '2': detailsTab = %d, want tabUses", um.detailsTab)
	}
	if cmd == nil {
		t.Error("after '2': expected non-nil load cmd, got nil")
	}

	// 3 (Where Used) and 4 (Drawings) shouldn't apply — current tab stays put.
	for _, key := range []rune{'3', '4'} {
		updated, cmd = um.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		nm := updated.(Model)
		if nm.detailsTab != tabUses {
			t.Errorf("after '%c': detailsTab changed from tabUses to %d", key, nm.detailsTab)
		}
		if cmd != nil {
			t.Errorf("after '%c': expected nil cmd, got %T", key, cmd)
		}
	}
}


// TestResetTabState_TerminatesAndZeroesAllFields catches a regression
// where a refactor accidentally called resetTabState() from inside
// itself (caught a real stack-overflow crash on hub-select). The test
// pre-populates every per-tab field, calls resetTabState, and asserts
// the call returns and all fields are zero. If anyone reintroduces
// recursion the goroutine stack overflows and the test fails.
func TestResetTabState_TerminatesAndZeroesAllFields(t *testing.T) {
	m := &Model{}
	m.tabLoading[tabUses] = true
	m.tabErr[tabWhereUsed] = "boom"
	m.tabCursors[tabDrawings] = 7
	m.tabScrolls[tabUses] = 3

	m.resetTabState()

	if m.tabLoading[tabUses] || m.tabErr[tabWhereUsed] != "" ||
		m.tabCursors[tabDrawings] != 0 || m.tabScrolls[tabUses] != 0 {
		t.Errorf("resetTabState left state populated: %+v", m)
	}
}

// TestUpdate_TabCursorMoves_OnNonDetailsTab confirms ↑/↓ moves the
// per-tab cursor (not the Contents column cursor) when a non-Details
// tab is active and rows are loaded.
func TestUpdate_TabCursorMoves_OnNonDetailsTab(t *testing.T) {
	m := designModelWithTabs()
	m.detailsTab = tabUses
	m.usesCache["CV1"] = []api.ComponentRef{
		{Name: "BoltA", DesignItemID: "di-bolt-a"},
		{Name: "BoltB", DesignItemID: "di-bolt-b"},
		{Name: "BoltC", DesignItemID: "di-bolt-c"},
	}

	// Initial cursor is 0; ↓ moves to 1, ↓ again to 2.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.tabCursors[tabUses] != 1 {
		t.Errorf("after ↓: tabCursors[tabUses] = %d, want 1", m.tabCursors[tabUses])
	}
	// Contents cursor must NOT move.
	if m.cursors[colContents] != 0 {
		t.Errorf("Contents cursor should stay put on non-Details tab: %d", m.cursors[colContents])
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.tabCursors[tabUses] != 2 {
		t.Errorf("after second ↓: tabCursors[tabUses] = %d, want 2", m.tabCursors[tabUses])
	}

	// Past last row: clamp at len-1.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.tabCursors[tabUses] != 2 {
		t.Errorf("clamped: tabCursors[tabUses] = %d, want 2", m.tabCursors[tabUses])
	}

	// ↑ moves back.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.tabCursors[tabUses] != 1 {
		t.Errorf("after ↑: tabCursors[tabUses] = %d, want 1", m.tabCursors[tabUses])
	}
}

// TestUpdate_EnterOnTabRow_DispatchesLocate confirms Enter on a non-Details
// tab fires the locate command for the highlighted row's DesignItemID.
func TestUpdate_EnterOnTabRow_DispatchesLocate(t *testing.T) {
	m := designModelWithTabs()
	m.detailsTab = tabWhereUsed
	m.whereUsedCache["CV1"] = []api.ComponentRef{
		{Name: "Robot", DesignItemID: "di-robot", DesignItemName: "Robot.f3d"},
		{Name: "Arm", DesignItemID: "di-arm", DesignItemName: "Arm.f3d"},
	}
	m.tabCursors[tabWhereUsed] = 1 // pretend the user moved cursor to row 1

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected non-nil locate cmd, got nil")
	}
	// Status message should reflect the in-flight locate.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um := updated.(Model)
	if !strings.Contains(um.statusMsg, "Locating") {
		t.Errorf("statusMsg = %q, want 'Locating ...'", um.statusMsg)
	}
}

// TestHandleItemLocation_CrossHub surfaces a friendly status when the
// located item is in a different hub than the current selection.
func TestHandleItemLocation_CrossHub(t *testing.T) {
	m := designModelWithTabs()
	m.selectedHubID = "H1"
	loc := &api.ItemLocation{
		HubID:       "H2",
		ProjectID:   "P-other",
		ProjectName: "ConfidentialProj",
	}
	updated, cmd := m.handleItemLocation(itemLocationLoadedMsg{loc: loc, target: "I1"})
	if cmd != nil {
		t.Errorf("cross-hub locate should not emit a command, got %T", cmd)
	}
	if !strings.Contains(updated.statusMsg, "another hub") {
		t.Errorf("statusMsg = %q, want substring 'another hub'", updated.statusMsg)
	}
}

// TestHandleItemLocation_ProjectNotVisible surfaces a friendly status
// when the located project isn't in the current hub's project list.
func TestHandleItemLocation_ProjectNotVisible(t *testing.T) {
	m := designModelWithTabs()
	loc := &api.ItemLocation{
		HubID:       "H1", // same as selectedHubID in designModelWithTabs
		ProjectID:   "P-stranger",
		ProjectName: "Restricted",
	}
	updated, cmd := m.handleItemLocation(itemLocationLoadedMsg{loc: loc, target: "I1"})
	if cmd != nil {
		t.Errorf("missing-project locate should not emit a command, got %T", cmd)
	}
	if !strings.Contains(updated.statusMsg, "not visible") {
		t.Errorf("statusMsg = %q, want substring 'not visible'", updated.statusMsg)
	}
}

// TestHandleItemLocation_DrillsFolders confirms the happy path:
// project is visible, folder chain is queued via pendingNav, and the
// resulting tea.Cmd loads project contents to start the drill.
func TestHandleItemLocation_DrillsFolders(t *testing.T) {
	m := designModelWithTabs()
	m.cols[colProjects] = []api.NavItem{
		{ID: "P-other", Name: "Other"},
		{ID: "P1", Name: "Match", Kind: "project", IsContainer: true},
	}
	loc := &api.ItemLocation{
		HubID:        "H1",
		ProjectID:    "P1",
		ProjectName:  "Match",
		ProjectAltID: "a.match",
		FolderPath: []api.FolderRef{
			{ID: "F-root", Name: "Top"},
			{ID: "F-leaf", Name: "Sub"},
		},
	}
	updated, cmd := m.handleItemLocation(itemLocationLoadedMsg{loc: loc, target: "di-target"})
	if cmd == nil {
		t.Fatal("expected loadProjectContentsCmd, got nil")
	}
	if updated.cursors[colProjects] != 1 {
		t.Errorf("project cursor = %d, want 1 (P1)", updated.cursors[colProjects])
	}
	if updated.activeCol != colContents {
		t.Errorf("activeCol = %d, want colContents", updated.activeCol)
	}
	if updated.pendingNav == nil {
		t.Fatal("pendingNav not set")
	}
	if updated.pendingNav.targetItemID != "di-target" {
		t.Errorf("pendingNav.targetItemID = %q, want 'di-target'", updated.pendingNav.targetItemID)
	}
	if len(updated.pendingNav.folders) != 2 {
		t.Errorf("pendingNav.folders len = %d, want 2", len(updated.pendingNav.folders))
	}
}

// TestUpdate_UsesLoadedMsg_PopulatesCacheAndClearsLoading drives a
// successful usesLoadedMsg through the model after a tabUses fetch was
// in flight, asserting the cache is populated and tabLoading[tabUses]
// is cleared so the renderer drops the spinner.
func TestUpdate_UsesLoadedMsg_PopulatesCacheAndClearsLoading(t *testing.T) {
	m := designModelWithTabs()
	m.detailsTab = tabUses
	m.tabLoading[tabUses] = true

	items := []api.ComponentRef{{Name: "BoltA", PartNumber: "PN-1"}}
	updated, cmd := m.Update(usesLoadedMsg{cvid: "CV1", items: items, err: nil})
	um := updated.(Model)
	if cmd != nil {
		t.Errorf("expected nil cmd after data arrives, got %T", cmd)
	}
	if um.tabLoading[tabUses] {
		t.Errorf("tabLoading[tabUses] still true after data arrived")
	}
	got, ok := um.usesCache["CV1"]
	if !ok {
		t.Fatal("usesCache missing entry for CV1")
	}
	if len(got) != 1 || got[0].PartNumber != "PN-1" {
		t.Errorf("cached items = %+v, want one BoltA / PN-1", got)
	}
}

// ---------------------------------------------------------------------------
// Async assembly-vs-part classification
// ---------------------------------------------------------------------------

// TestUpdate_ItemClassified_UpdatesSubtype confirms that a fresh
// itemClassifiedMsg whose gen matches m.contentsGen stamps the matching
// row's Subtype to "assembly" or "part".
func TestUpdate_ItemClassified_UpdatesSubtype(t *testing.T) {
	m := Model{
		state:        stateBrowsing,
		width:        120,
		height:       40,
		contentsGen: 3,
		cols: [numCols][]api.NavItem{
			{},
			{
				{ID: "d1", Name: "Assembly", Kind: "design"},
				{ID: "d2", Name: "Part", Kind: "design"},
			},
		},
		spinner:      spinner.New(),
		styleCache:   &styleCache{},
		detailsCache: map[string]*api.ItemDetails{},
	}

	// Mark d1 as an assembly.
	updated, _ := m.Update(itemClassifiedMsg{gen: 3, itemID: "d1", isAssembly: true})
	m1 := updated.(Model)
	if got := m1.cols[colContents][0].Subtype; got != "assembly" {
		t.Errorf("d1 Subtype = %q, want assembly", got)
	}
	if got := m1.cols[colContents][1].Subtype; got != "" {
		t.Errorf("d2 Subtype should be unchanged, got %q", got)
	}

	// Mark d2 as a part.
	updated, _ = m1.Update(itemClassifiedMsg{gen: 3, itemID: "d2", isAssembly: false})
	m2 := updated.(Model)
	if got := m2.cols[colContents][1].Subtype; got != "part" {
		t.Errorf("d2 Subtype = %q, want part", got)
	}
}

// TestUpdate_ItemClassified_StaleGenDropped confirms that an
// itemClassifiedMsg dispatched against a previous folder selection is
// silently ignored when m.contentsGen has moved on. This is the
// cancellation primitive for the parallel-classify pipeline.
func TestUpdate_ItemClassified_StaleGenDropped(t *testing.T) {
	m := Model{
		state:       stateBrowsing,
		width:       120,
		height:      40,
		contentsGen: 5, // current
		cols: [numCols][]api.NavItem{
			{},
			{{ID: "d1", Name: "Design", Kind: "design"}},
		},
		spinner:      spinner.New(),
		styleCache:   &styleCache{},
		detailsCache: map[string]*api.ItemDetails{},
	}

	updated, _ := m.Update(itemClassifiedMsg{gen: 2 /* stale */, itemID: "d1", isAssembly: true})
	um := updated.(Model)
	if got := um.cols[colContents][0].Subtype; got != "" {
		t.Errorf("stale-gen msg should not mutate Subtype, got %q", got)
	}
}

// TestUpdate_ItemClassified_ErrorDropped: a per-row classify failure
// keeps the generic design icon (graceful degradation) rather than
// surfacing as a user-visible error. The renderer treats an empty
// Subtype on a design as "not yet classified" — same as in-flight.
func TestUpdate_ItemClassified_ErrorDropped(t *testing.T) {
	m := Model{
		state:       stateBrowsing,
		width:       120,
		height:      40,
		contentsGen: 1,
		cols: [numCols][]api.NavItem{
			{},
			{{ID: "d1", Name: "Design", Kind: "design"}},
		},
		spinner:      spinner.New(),
		styleCache:   &styleCache{},
		detailsCache: map[string]*api.ItemDetails{},
	}

	updated, _ := m.Update(itemClassifiedMsg{gen: 1, itemID: "d1", err: errors.New("flaky")})
	um := updated.(Model)
	if got := um.cols[colContents][0].Subtype; got != "" {
		t.Errorf("err msg should not mutate Subtype, got %q", got)
	}
}

// TestClassifyContentsCmd_OnlyDesigns asserts that the fan-out cmd
// skips non-designs and unclassifiable designs (no componentVersion
// id). Drawings, folders, and already-classified rows must not produce
// extra round-trips.
func TestClassifyContentsCmd_OnlyDesigns(t *testing.T) {
	items := []api.NavItem{
		{ID: "d1", Kind: "design", ComponentVersionID: "cv1"},   // classify
		{ID: "d2", Kind: "design", ComponentVersionID: ""},      // skip (no cv)
		{ID: "d3", Kind: "design", ComponentVersionID: "cv3", Subtype: "part"}, // skip (already classified)
		{ID: "dw", Kind: "drawing"},                              // skip (non-design)
		{ID: "f1", Kind: "folder", IsContainer: true},            // skip (container)
	}
	cmd := classifyContentsCmd("tok", items, 1)
	if cmd == nil {
		t.Fatal("expected a batch cmd when at least one design is classifiable, got nil")
	}

	// All non-classifiables should also short-circuit to nil.
	if classifyContentsCmd("tok", []api.NavItem{
		{ID: "dw", Kind: "drawing"},
		{ID: "f1", Kind: "folder", IsContainer: true},
	}, 1) != nil {
		t.Errorf("expected nil cmd when no designs need classifying")
	}
}

// TestSubtypeSuffix covers the inline-tag display for every supported
// Kind/Subtype combo, including the new Fusion Electronics types
// (schematic / pcb / ecad) and the drawing template/dwg split.
func TestSubtypeSuffix(t *testing.T) {
	cases := []struct {
		name string
		item api.NavItem
		want string
	}{
		{"design unclassified", api.NavItem{Kind: "design"}, ""},
		{"design assembly", api.NavItem{Kind: "design", Subtype: "assembly"}, "  · asm"},
		{"design part", api.NavItem{Kind: "design", Subtype: "part"}, "  · part"},
		{"drawing dwg", api.NavItem{Kind: "drawing", Subtype: "dwg"}, "  · dwg"},
		{"drawing template", api.NavItem{Kind: "drawing", Subtype: "template"}, "  · template"},
		{"drawing unset is no suffix", api.NavItem{Kind: "drawing"}, ""},
		{"pcb", api.NavItem{Kind: "pcb"}, "  · pcb"},
		{"schematic", api.NavItem{Kind: "schematic"}, "  · schem"},
		{"ecad", api.NavItem{Kind: "ecad"}, "  · ecad"},
		{"configured has no suffix", api.NavItem{Kind: "configured"}, ""},
		{"folder has no suffix", api.NavItem{Kind: "folder"}, ""},
		{"project has no suffix", api.NavItem{Kind: "project"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := subtypeSuffix(tc.item); got != tc.want {
				t.Errorf("subtypeSuffix(%+v) = %q, want %q", tc.item, got, tc.want)
			}
		})
	}
}
