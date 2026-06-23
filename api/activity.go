package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// feedBaseURL is the root of the (undocumented, first-party) Fusion Team
// notifications service the web client uses for its activity feed. It is a var
// (not const) so tests can point it at an httptest.Server; production never
// reassigns it. See docs/activity-reports/feed-contract.md.
var feedBaseURL = "https://developer.api.autodesk.com/fusionteam/notifications/v2"

// SetFeedBaseURLForTesting overrides the notifications base URL and returns a
// restore func. Tests only; production MUST NOT call this.
func SetFeedBaseURLForTesting(u string) (restore func()) {
	prev := feedBaseURL
	feedBaseURL = u
	return func() { feedBaseURL = prev }
}

const (
	// feedPageSize is the page size requested per call. The web client uses 40;
	// the loop tolerates whatever page size the server actually returns.
	feedPageSize = 100
	// feedMaxPages is a hard backstop so a misbehaving "nextPage" link can never
	// spin forever (100 pages * 100 = 10k events, far beyond any real hub feed).
	feedMaxPages = 100
)

// Activity action verbs (inferred — the feed JSON carries no explicit verb).
const (
	ActionCreated   = "created"
	ActionUpdated   = "updated"
	ActionCommunity = "community" // lifecycle events (project created, etc.)
)

// Actor identifies who performed an activity.
type Actor struct {
	AccountID   string `json:"accountId,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
}

// ActivityEvent is one normalized entry from the activity feed. Every event
// carries the full hub -> project -> folder -> design hierarchy, so the
// aggregation layer can group it at any scope without extra round-trips.
type ActivityEvent struct {
	// EntityType is the kind of thing the event is about: "design" for file
	// versions, "community" for lifecycle events (e.g. project created).
	EntityType string    `json:"entityType"`
	EntityID   string    `json:"entityId"`   // permalinkId / object id
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
	Comments int `json:"comments,omitempty"` // postCount
	Likes    int `json:"likes,omitempty"`

	// Detail holds extra human-readable context (e.g. the COMMUNITY event text).
	Detail string `json:"detail,omitempty"`

	Source string `json:"source"` // "feed"
}

// GetActivityFeed fetches and normalizes the full network activity feed for a
// hub, following pagination until the server stops advertising a next page.
// hubID is the short hub slug (e.g. "imallc"), not the a.* Data Management id.
func GetActivityFeed(ctx context.Context, token, hubID string) ([]ActivityEvent, error) {
	if hubID == "" {
		return nil, fmt.Errorf("activity: empty hubID")
	}
	var events []ActivityEvent
	seen := make(map[string]struct{})
	for page := 1; page <= feedMaxPages; page++ {
		start := (page - 1) * feedPageSize
		resp, err := fetchFeedPage(ctx, token, hubID, start, feedPageSize, page)
		if err != nil {
			return nil, err
		}
		for _, o := range resp.Objects {
			ev := o.normalize()
			// Guard against page overlap (pages should be disjoint, but the
			// feed is volatile while edits land).
			key := ev.EntityID + "|" + ev.Timestamp.Format(time.RFC3339Nano) + "|" + strconv.Itoa(ev.VersionNumber)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			events = append(events, ev)
		}
		if len(resp.Objects) == 0 || !feedHasNextPage(resp.Links.Link) {
			break
		}
	}
	return events, nil
}

func fetchFeedPage(ctx context.Context, token, hubID string, start, count, page int) (*feedResponse, error) {
	u := fmt.Sprintf("%s/hubs/%s/feeds/network/@me?count=%d&start=%d&page=%d",
		feedBaseURL, url.PathEscape(hubID), count, start, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if region != "" {
		req.Header.Set("X-Ads-Region", region)
	}

	dbgLog("ACTIVITY REQUEST %s", u)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("activity feed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("activity feed read: %w", err)
	}
	dbgLog("ACTIVITY RESPONSE HTTP %d (%d bytes)", resp.StatusCode, len(raw))

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("activity feed unauthorized (HTTP 401) — token may be expired or lacks scope/entitlement")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("activity feed HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var fr feedResponse
	if err := json.Unmarshal(raw, &fr); err != nil {
		return nil, fmt.Errorf("activity feed decode: %w", err)
	}
	return &fr, nil
}

// --- raw feed wire types (numeric fields arrive as JSON strings) ---

type feedResponse struct {
	StartIndex   string       `json:"startIndex"`
	Count        string       `json:"count"`
	TotalObjects string       `json:"totalObjects"`
	Objects      []feedObject `json:"objects"`
	Links        struct {
		// In the envelope this is a single object; within per-item payloads it
		// is an array. Decode raw and probe both shapes (see feedHasNextPage).
		Link json.RawMessage `json:"link"`
	} `json:"links"`
}

type feedObject struct {
	AtType          string        `json:"@type"`
	Type            string        `json:"type"` // DATA | COMMUNITY
	ID              string        `json:"id"`
	PermalinkID     string        `json:"permalinkId"`
	DisplayTitle    string        `json:"displayTitle"`
	FileName        string        `json:"fileName"`
	FileType        string        `json:"fileType"`
	PermalinkURL    string        `json:"permalinkUrl"`
	CreationTime    string        `json:"creationTime"`
	LastModified    string        `json:"lastModified"`
	ChangeTime      string        `json:"changeTime"`
	Version         string        `json:"version"`
	LineageURN      string        `json:"lineageUrn"`
	TipVersionURN   string        `json:"tipVersionUrn"`
	ParentFolderURN string        `json:"parentFolderUrn"`
	PostCount       string        `json:"postCount"`
	LikeCount       string        `json:"likeCount"`
	Owner           feedUser      `json:"owner"`
	LastActivity    feedActor     `json:"lastActivity"`
	LastUpdate      feedActor     `json:"lastUpdate"`
	PublishedTo     feedGroup     `json:"publishedTo"`
	Hub             feedHub       `json:"hub"`
	Views           feedViews     `json:"views"`
	Title           feedHTMLField `json:"title"`
}

type feedUser struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
	UserID      string `json:"userId"` // email
}

type feedActor struct {
	Time        string `json:"time"`
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
}

type feedGroup struct {
	Type            string `json:"type"`
	ID              string `json:"id"`
	PublishedToName string `json:"publishedToName"`
	PublishedToURL  string `json:"publishedToUrl"`
}

type feedHub struct {
	Name    string `json:"name"`
	HubID   string `json:"hubId"`
	ForgeID string `json:"forgeId"`
}

type feedViews struct {
	Views   string `json:"views"`
	Viewers string `json:"viewers"`
}

type feedHTMLField struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// normalize converts a raw feed object into an ActivityEvent.
func (o feedObject) normalize() ActivityEvent {
	name := o.DisplayTitle
	if name == "" {
		name = o.FileName
	}
	id := o.PermalinkID
	if id == "" {
		id = o.ID
	}
	ev := ActivityEvent{
		EntityID:      id,
		EntityName:    name,
		Timestamp:     firstTime(parseEpochMillis(o.ChangeTime), parseEpochMillis(o.LastActivity.Time), parseEpochMillis(o.LastModified), parseEpochMillis(o.CreationTime)),
		VersionNumber: atoiSafe(o.Version),
		HubID:         o.Hub.HubID,
		HubName:       o.Hub.Name,
		HubForgeID:    o.Hub.ForgeID,
		ProjectID:     o.PublishedTo.ID,
		ProjectName:   o.PublishedTo.PublishedToName,
		FolderURN:     o.ParentFolderURN,
		LineageURN:    o.LineageURN,
		FileType:      o.FileType,
		WebURL:        o.PermalinkURL,
		CreatedOn:     parseEpochMillis(o.CreationTime),
		Owner:         Actor{AccountID: o.Owner.AccountID, DisplayName: o.Owner.DisplayName, Email: o.Owner.UserID},
		Views:         atoiSafe(o.Views.Views),
		Comments:      atoiSafe(o.PostCount),
		Likes:         atoiSafe(o.LikeCount),
		Source:        "feed",
	}
	ev.Actor = o.bestActor()

	if o.Type == "COMMUNITY" || o.AtType == "activityFeedDataObject" {
		ev.EntityType = "community"
		ev.Action = ActionCommunity
		ev.Detail = stripHTML(firstNonEmpty(o.Title.Content, o.DisplayTitle))
		if t := parseEpochMillis(o.CreationTime); !t.IsZero() {
			ev.Timestamp = t
		}
		return ev
	}

	ev.EntityType = "design"
	if ev.VersionNumber <= 1 {
		ev.Action = ActionCreated
	} else {
		ev.Action = ActionUpdated
	}
	return ev
}

// bestActor picks the most specific last-actor available, falling back to owner.
func (o feedObject) bestActor() Actor {
	if o.LastActivity.DisplayName != "" || o.LastActivity.AccountID != "" {
		return Actor{AccountID: o.LastActivity.AccountID, DisplayName: o.LastActivity.DisplayName}
	}
	if o.LastUpdate.DisplayName != "" || o.LastUpdate.AccountID != "" {
		return Actor{AccountID: o.LastUpdate.AccountID, DisplayName: o.LastUpdate.DisplayName}
	}
	return Actor{AccountID: o.Owner.AccountID, DisplayName: o.Owner.DisplayName, Email: o.Owner.UserID}
}

// --- small parse helpers ---

// parseEpochMillis parses a string of epoch milliseconds (the feed's time
// format). Empty / "0" / unparseable yields the zero time.
func parseEpochMillis(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return time.Time{}
	}
	ms, err := strconv.ParseInt(s, 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func firstTime(ts ...time.Time) time.Time {
	for _, t := range ts {
		if !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// HubSlug derives the short hub slug (e.g. "imallc") the notifications feed
// needs from the identifiers the GraphQL hub list already provides: the Data
// Management hub id (`a.` + base64("business:<slug>")) or, failing that, the
// subdomain of the fusion web URL (https://<slug>.autodesk360.com/…). Returns
// "" if neither yields a usable slug.
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

var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

// stripHTML removes tags and unescapes entities for a plain-text rendering of
// COMMUNITY event titles (which arrive as HTML anchors).
func stripHTML(s string) string {
	if s == "" {
		return ""
	}
	s = htmlTagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

// feedHasNextPage reports whether the envelope's links advertise a next page.
// The "link" value is a single object in the envelope but an array elsewhere,
// so both shapes are probed.
func feedHasNextPage(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	type rel struct {
		Rel string `json:"rel"`
	}
	var one rel
	if json.Unmarshal(raw, &one) == nil && one.Rel != "" {
		return strings.EqualFold(one.Rel, "nextPage")
	}
	var many []rel
	if json.Unmarshal(raw, &many) == nil {
		for _, l := range many {
			if strings.EqualFold(l.Rel, "nextPage") {
				return true
			}
		}
	}
	return false
}
