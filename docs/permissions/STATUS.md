# Permissions tab — status

The Details **Permissions** tab is a Permission Explorer over the real
Manufacturing Data Model permission surface. Branch: `feature/activity-reports`
(landed with the activity work; not yet PR'd to main). Schema + Fusion semantics
are captured in the auto-memory `mdm-permission-model`.

## Done

- **Backend** `GET /api/permissions/path?hubId&projectId&folderId…` (`api/permissions.go`,
  `server/handlers_perms.go`) — returns access at each layer of a document's path:
  the project (`groups` + `folderLevelProjectMembers`) and each folder (`members` +
  `groups`), root→leaf, fetched concurrently. `folder.members` is **effective**
  (includes inherited grants), so the leaf layer is exactly who can reach the document.
  `GetProjectMembers` / `GetFolderMembers` / `GetFolderGroups` added.
- **Frontend** (`web/src/components/PermissionsExplorer.tsx`) — resolves the
  per-principal cascade across layers (directly applied / inherited / raised / lowered /
  **denied** = Fusion "No role"). **With access** lists groups + individual members
  (project contributors + folder members; `PENDING` → Invited chip) by effective role,
  plus a **Denied here** section. **Path layers** has two styles — a Layers spine
  (default) and concentric Circles — that trace the hovered principal. Roles use the
  real `FolderRoleEnum` ladder (Viewer → Reader → Editor → Manager → Administrator).

## Future explorations

1. **Project-wide access matrix / "no access" roster.** The per-document tab only needs
   the path layers. To answer "who in the whole project has *no* access to this document"
   we need the full project population = union of members across the project + **every**
   folder (a recursive folder walk, like `GetAllDescendants` for assemblies — bounded and
   cacheable per project). Then per document: no-access = population − the doc's effective
   members. Basis for a separate project-wide access-matrix view.

2. **Group → member expansion without hub-admin.** Individual project/folder members are
   readable by any project member, but expanding a *group* to its users
   (`GetGroupMembers`) still requires hub-admin (else 403). Explore whether another query
   path exposes group membership to ordinary members, or surface the limitation better
   (e.g. show the group's member count even when the roster isn't listable).
