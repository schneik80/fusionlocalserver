package api

import (
	"encoding/base64"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Activity model shared by the design-scope activity report. The Fusion Team
// notifications feed this once fed is first-party-gated (returns HTTP 500 for
// this app's token), so activity is sourced from the Manufacturing Data Model
// GraphQL instead — see api/activity_graphql.go. HubSlug is retained because the
// nav still derives a hub slug for display (server/dto.go).

// Activity action verbs (inferred — there is no explicit verb in the data).
const (
	ActionCreated = "created"
	ActionUpdated = "updated"
)

// Actor identifies who performed an activity.
type Actor struct {
	AccountID   string `json:"accountId,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
}

// ActivityEvent is one normalized activity entry. For a design report each
// version becomes one event (api/activity_graphql.go); the lineage/hierarchy
// fields let BuildReport filter and roll up.
type ActivityEvent struct {
	// EntityType is the kind of thing the event is about: "design" for file
	// versions, "community" for lifecycle events.
	EntityType string    `json:"entityType"`
	EntityID   string    `json:"entityId"`   // permalinkId / lineage id
	EntityName string    `json:"entityName"` // displayTitle / fileName
	Timestamp  time.Time `json:"timestamp"`  // when the change happened (absolute)
	Action     string    `json:"action"`
	Actor      Actor     `json:"actor"` // who made this change (last actor)

	VersionNumber int `json:"versionNumber,omitempty"`

	// Lineage / hierarchy
	HubID       string `json:"hubId,omitempty"`
	HubName     string `json:"hubName,omitempty"`
	HubForgeID  string `json:"hubForgeId,omitempty"` // a.* Data Management hub id
	ProjectID   string `json:"projectId,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
	FolderURN   string `json:"folderUrn,omitempty"`
	LineageURN  string `json:"lineageUrn,omitempty"`
	FileType    string `json:"fileType,omitempty"`
	WebURL      string `json:"webUrl,omitempty"`

	// Lineage creation (carried so design-scope aggregation can compute the
	// "created" date/author independent of this event's timestamp).
	CreatedOn time.Time `json:"createdOn,omitempty"`
	Owner     Actor     `json:"owner,omitempty"`

	// Bonus signals
	Views    int `json:"views,omitempty"`
	Comments int `json:"comments,omitempty"`
	Likes    int `json:"likes,omitempty"`

	// Detail holds extra human-readable context.
	Detail string `json:"detail,omitempty"`

	Source string `json:"source"` // "graphql"
}

// HubSlug derives the short hub slug (e.g. "imallc") from the identifiers the
// GraphQL hub list provides: the Data Management hub id (`a.` + base64(
// "business:<slug>")) or, failing that, the subdomain of the fusion web URL
// (https://<slug>.autodesk360.com/…). Returns "" if neither yields a slug.
func HubSlug(altID, webURL string) string {
	if s := slugFromAltID(altID); s != "" {
		return s
	}
	return slugFromURL(webURL)
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func slugFromAltID(altID string) string {
	a := altID
	if len(a) > 2 && a[1] == '.' { // strip "a." / "b." style prefix
		a = a[2:]
	}
	dec, err := base64.StdEncoding.DecodeString(a)
	if err != nil {
		dec, err = base64.RawStdEncoding.DecodeString(a)
	}
	if err != nil {
		return ""
	}
	s := string(dec)
	if i := strings.LastIndex(s, ":"); i >= 0 && i+1 < len(s) {
		s = s[i+1:]
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if slugRE.MatchString(s) {
		return s
	}
	return ""
}

func slugFromURL(webURL string) string {
	if webURL == "" {
		return ""
	}
	u, err := url.Parse(webURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := u.Hostname()
	label := host
	if i := strings.Index(host, "."); i > 0 {
		label = host[:i]
	}
	label = strings.ToLower(label)
	if label == "" || label == "www" || !slugRE.MatchString(label) {
		return ""
	}
	return label
}
