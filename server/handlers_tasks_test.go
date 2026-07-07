package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/schneik80/fusionlocalserver/api"
	"github.com/schneik80/fusionlocalserver/chat"
	"github.com/schneik80/fusionlocalserver/internal/testutil"
	"github.com/schneik80/fusionlocalserver/tasks"
)

const (
	taskTestProject  = "urn:project:tasks-1"
	taskTestProject2 = "urn:project:tasks-2"
)

// newTaskTestServer builds a Server with a real tasks store over a TempDir
// and the shared chat authorizer pointed at a fake APS roster (the chat
// fixture's cast: one VIEWER, one EDITOR, one MANAGER).
func newTaskTestServer(t *testing.T) *Server {
	t.Helper()
	roster := &fakeRoster{rows: []map[string]any{
		rosterRow("u-viewer", "viewer@x.io", "VIEWER"),
		rosterRow("u-editor", "editor@x.io", "EDITOR"),
		rosterRow("u-manager", "manager@x.io", "MANAGER"),
	}}
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		return testutil.GraphQLResponse{Data: map[string]any{
			"project": map[string]any{
				"folderLevelProjectMembers": map[string]any{
					"pagination": map[string]any{"cursor": ""},
					"results":    roster.snapshot(),
				},
			},
		}}
	})
	restore := api.SetGraphqlEndpointForTesting(srv.URL)
	t.Cleanup(restore)

	store, err := tasks.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		logger:    quietLogger(),
		clientID:  "test-client",
		sessions:  NewSessionStore(sessionIdleTTL, sessionAbsTTL, quietLogger()),
		pending:   NewPendingStore(pendingTTL),
		tasks:     store,
		chatAuthz: chat.NewAuthorizer(),
		taskOpLim: chat.NewLimiter(50, 100), // roomy: tests mutate rapidly
	}
}

func taskURL(path string, kv ...string) string {
	q := "projectId=" + taskTestProject
	for i := 0; i+1 < len(kv); i += 2 {
		q += "&" + kv[i] + "=" + kv[i+1]
	}
	return path + "?" + q
}

func taskCreateBody(title string) map[string]any {
	return map[string]any{"hubId": "hub-1", "projectName": "Test Project", "title": title}
}

func TestTasks_RequiresSessionAndRole(t *testing.T) {
	s := newTaskTestServer(t)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)

	if code := chatDo(t, ts.URL, http.MethodGet, taskURL("/api/tasks"), nil, nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", code)
	}

	outsider := login(t, s, "u-outsider", "Out Sider", "out@x.io")
	// The fake roster answers for any project, so an unlisted user is
	// "roster readable but not listed" → group-derived write. To get a
	// denial we need someone the roster lists as suspended.
	viewer := login(t, s, "u-viewer", "Vera Viewer", "viewer@x.io")

	// Viewer can read but not create.
	var list TaskListDTO
	if code := chatDo(t, ts.URL, http.MethodGet, taskURL("/api/tasks"), viewer, nil, &list); code != http.StatusOK {
		t.Fatalf("viewer list status = %d", code)
	}
	if list.Tasks == nil || list.Capabilities.Write {
		t.Fatalf("viewer list = %+v; want empty non-nil tasks and write=false", list)
	}
	if code := chatDo(t, ts.URL, http.MethodPost, taskURL("/api/tasks"), viewer, taskCreateBody("nope"), nil); code != http.StatusForbidden {
		t.Fatalf("viewer create status = %d, want 403", code)
	}
	_ = outsider
}

func TestTasks_CreatePatchDeleteRoundTrip(t *testing.T) {
	s := newTaskTestServer(t)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	editor := login(t, s, "u-editor", "Ed Editor", "editor@x.io")
	manager := login(t, s, "u-manager", "Man Ager", "manager@x.io")

	// Create.
	body := taskCreateBody("Ship the tasks feature")
	body["assignee"] = map[string]any{"id": "u-editor", "name": "Ed Editor", "email": "editor@x.io"}
	body["docRefs"] = []string{"fls:doc?hubId=h&itemId=i1"}
	var created TaskDTO
	if code := chatDo(t, ts.URL, http.MethodPost, taskURL("/api/tasks"), editor, body, &created); code != http.StatusCreated {
		t.Fatalf("create status = %d", code)
	}
	if created.ID != "t1" || created.Num != 1 || created.Status != "todo" || created.Priority != "medium" {
		t.Fatalf("created = %+v", created)
	}
	if created.HubID != "hub-1" || created.ProjectName != "Test Project" || created.ProjectID != taskTestProject {
		t.Fatalf("project annotation wrong: %+v", created)
	}
	if created.CreatedBy.ID != "u-editor" || created.CreatedBy.Name != "Ed Editor" {
		t.Fatalf("createdBy = %+v", created.CreatedBy)
	}

	// Validation bounces.
	if code := chatDo(t, ts.URL, http.MethodPost, taskURL("/api/tasks"), editor, taskCreateBody(""), nil); code != http.StatusBadRequest {
		t.Fatalf("empty-title create status = %d, want 400", code)
	}
	noHub := map[string]any{"title": "x"}
	if code := chatDo(t, ts.URL, http.MethodPost, taskURL("/api/tasks"), editor, noHub, nil); code != http.StatusBadRequest {
		t.Fatalf("missing-hub create status = %d, want 400", code)
	}

	// Patch: status change + clear assignee.
	patch := map[string]any{"status": "done", "clearAssignee": true}
	var updated TaskDTO
	if code := chatDo(t, ts.URL, http.MethodPatch, taskURL("/api/tasks", "taskId", created.ID), editor, patch, &updated); code != http.StatusOK {
		t.Fatalf("patch status = %d", code)
	}
	if updated.Status != "done" || updated.Assignee != nil {
		t.Fatalf("patch not applied: %+v", updated)
	}

	// Unknown task → 404.
	if code := chatDo(t, ts.URL, http.MethodPatch, taskURL("/api/tasks", "taskId", "t99"), editor, patch, nil); code != http.StatusNotFound {
		t.Fatalf("patch missing status = %d, want 404", code)
	}
	if code := chatDo(t, ts.URL, http.MethodGet, taskURL("/api/tasks/get", "taskId", "t99"), editor, nil, nil); code != http.StatusNotFound {
		t.Fatalf("get missing status = %d, want 404", code)
	}

	// Get.
	var got TaskDTO
	if code := chatDo(t, ts.URL, http.MethodGet, taskURL("/api/tasks/get", "taskId", created.ID), editor, nil, &got); code != http.StatusOK {
		t.Fatalf("get status = %d", code)
	}
	if got.Title != "Ship the tasks feature" || len(got.DocRefs) != 1 {
		t.Fatalf("get = %+v", got)
	}

	// Delete: a second editor's task can't be deleted by a non-creator
	// editor, but a manager can.
	var second TaskDTO
	if code := chatDo(t, ts.URL, http.MethodPost, taskURL("/api/tasks"), manager, taskCreateBody("Manager's task"), &second); code != http.StatusCreated {
		t.Fatalf("manager create status = %d", code)
	}
	if code := chatDo(t, ts.URL, http.MethodDelete, taskURL("/api/tasks", "taskId", second.ID), editor, nil, nil); code != http.StatusForbidden {
		t.Fatalf("non-creator delete status = %d, want 403", code)
	}
	if code := chatDo(t, ts.URL, http.MethodDelete, taskURL("/api/tasks", "taskId", created.ID), manager, nil, nil); code != http.StatusOK {
		t.Fatalf("moderator delete status = %d", code)
	}
	if code := chatDo(t, ts.URL, http.MethodGet, taskURL("/api/tasks/get", "taskId", created.ID), editor, nil, nil); code != http.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", code)
	}
}

func TestTasks_Mine(t *testing.T) {
	s := newTaskTestServer(t)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	editor := login(t, s, "u-editor", "Ed Editor", "editor@x.io")
	manager := login(t, s, "u-manager", "Man Ager", "manager@x.io")

	if code := chatDo(t, ts.URL, http.MethodPost, taskURL("/api/tasks"), editor, taskCreateBody("mine by creation"), nil); code != http.StatusCreated {
		t.Fatal("create 1")
	}
	assigned := taskCreateBody("assigned to editor")
	assigned["assignee"] = map[string]any{"id": "u-editor", "email": "editor@x.io"}
	if code := chatDo(t, ts.URL, http.MethodPost, taskURL("/api/tasks"), manager, assigned, nil); code != http.StatusCreated {
		t.Fatal("create 2")
	}
	// A second project, unrelated to the editor.
	other := "/api/tasks?projectId=" + taskTestProject2
	if code := chatDo(t, ts.URL, http.MethodPost, other, manager, taskCreateBody("not editor's"), nil); code != http.StatusCreated {
		t.Fatal("create 3")
	}

	var mine MyTasksDTO
	if code := chatDo(t, ts.URL, http.MethodGet, "/api/tasks/mine", editor, nil, &mine); code != http.StatusOK {
		t.Fatalf("mine status = %d", code)
	}
	if len(mine.Tasks) != 2 {
		t.Fatalf("mine len = %d, want 2: %+v", len(mine.Tasks), mine.Tasks)
	}
	for _, task := range mine.Tasks {
		if task.ProjectID != taskTestProject || task.HubID != "hub-1" || task.ProjectName != "Test Project" {
			t.Fatalf("mine annotation wrong: %+v", task)
		}
	}

	var mgrMine MyTasksDTO
	if code := chatDo(t, ts.URL, http.MethodGet, "/api/tasks/mine", manager, nil, &mgrMine); code != http.StatusOK {
		t.Fatalf("manager mine status = %d", code)
	}
	if len(mgrMine.Tasks) != 2 {
		t.Fatalf("manager mine len = %d, want 2 (created two)", len(mgrMine.Tasks))
	}
}

func TestTasks_StoreUnavailable(t *testing.T) {
	s := newTaskTestServer(t)
	s.tasks = nil
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	editor := login(t, s, "u-editor", "Ed Editor", "editor@x.io")

	if code := chatDo(t, ts.URL, http.MethodGet, taskURL("/api/tasks"), editor, nil, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("list status = %d, want 503", code)
	}
	if code := chatDo(t, ts.URL, http.MethodGet, "/api/tasks/mine", editor, nil, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("mine status = %d, want 503", code)
	}
}
