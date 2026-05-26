package ui

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/schneik80/fusionlocalserver/api"
	"github.com/schneik80/fusionlocalserver/auth"
	"github.com/schneik80/fusionlocalserver/config"
	"github.com/schneik80/fusionlocalserver/fusion"
	"github.com/schneik80/fusionlocalserver/pins"
)

// ---------------------------------------------------------------------------
// App state
// ---------------------------------------------------------------------------

type appState int

const (
	stateSetupNeeded appState = iota // config file missing or incomplete
	stateLoading                     // checking saved tokens
	stateAuthNeeded                  // no token; prompt user to log in
	stateAuthWaiting                 // browser opened; waiting for callback
	stateBrowsing                    // main 2-column browser + details
	stateHubSelect                   // hub selection overlay
	stateAbout                       // about / license overlay
	stateDebug                       // debug log overlay
	statePins                        // pins overlay
	stateError                       // unrecoverable error
)

// Column indices (hubs are now an overlay, not a column)
const (
	colProjects = 0
	colContents = 1
	numCols     = 2
)

// Details-pane tabs. tabDetails is the existing metadata view; the other
// three are component-level cross-references on the selected design's
// tipRootComponentVersion. They are only meaningful for DesignItems with a
// populated RootComponentVersionID — for other item kinds the tab strip
// is hidden and only the Details view renders.
type detailsTab int

const (
	tabDetails detailsTab = iota
	tabUses
	tabWhereUsed
	tabDrawings
	numDetailsTabs
)

func (t detailsTab) label() string {
	switch t {
	case tabDetails:
		return "Details"
	case tabUses:
		return "Uses"
	case tabWhereUsed:
		return "Where Used"
	case tabDrawings:
		return "Drawings"
	}
	return "?"
}

func (t detailsTab) labelShort() string {
	switch t {
	case tabDetails:
		return "Det"
	case tabUses:
		return "Uses"
	case tabWhereUsed:
		return "WUsed"
	case tabDrawings:
		return "Dwg"
	}
	return "?"
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type (
	tokenReadyMsg      struct{ token string }
	hubsLoadedMsg      struct{ items []api.NavItem }
	projectsLoadedMsg  struct{ items []api.NavItem }
	contentsLoadedMsg  struct{ items []api.NavItem }
	detailsLoadedMsg   struct{ details *api.ItemDetails }
	// usesLoadedMsg / whereUsedLoadedMsg / drawingsLoadedMsg deliver the
	// async result of a per-tab fetch. cvid identifies which component
	// version the data is for, so a stale response that arrives after the
	// user has navigated away can still be cached without overwriting
	// state for the new selection.
	usesLoadedMsg      struct{ cvid string; items []api.ComponentRef; err error }
	whereUsedLoadedMsg struct{ cvid string; items []api.ComponentRef; err error }
	// drawingsLoadedMsg key is the DesignItem.id (lineage URN), not a
	// component-version id — drawings are aggregated across all versions
	// of the design, so we cache by the stable item identity.
	drawingsLoadedMsg  struct{ itemID string; items []api.DrawingRef;   err error }
	// itemLocationLoadedMsg is the result of GetItemLocation, used to
	// orchestrate "Show in Location" navigation from the Uses /
	// Where Used / Drawings tabs into the Contents column.
	itemLocationLoadedMsg struct {
		loc    *api.ItemLocation
		target string // original requested itemID — used to position cursor at end
		err    error
	}
	errMsg            struct{ err error }
	// openedBrowserMsg reports the URL that was handed to the OS browser
	// handler so the status bar can display it (useful when the target
	// page errors out — e.g. Autodesk's "WEB SESSION INVALID" response —
	// so the user can see the exact URL and copy it manually).
	openedBrowserMsg   struct{ url string }
	// fusionActionMsg is the result of an asynchronous open/insert call
	// against the local Fusion MCP server. If err is non-nil, the status bar
	// shows the error; otherwise it shows the action string.
	fusionActionMsg struct {
		action string
		err    error
	}
	// stepStatusMsg reports the current state of a STEP derivative
	// generation request. Either err is set (transport failure) or status
	// is one of api.StepStatus*. When status is SUCCESS, signedURL is the
	// pre-authenticated download URL.
	stepStatusMsg struct {
		status    string
		signedURL string
		err       error
		// cvid + name are echoed back so the Update handler can keep
		// polling / continue with the download without having to look
		// the design back up in the model (which may have moved on).
		cvid string
		name string
	}
	// stepDoneMsg fires after the STEP file has been written to disk
	// (path) or the download/translation has failed (err).
	stepDoneMsg struct {
		path string
		err  error
	}
	// itemClassifiedMsg refines a single Contents-column row's design
	// subtype (assembly vs part) after the async ClassifyAssembly probe
	// returns. gen carries the contentsGen value at dispatch time so
	// late-arriving refinements that belong to a folder the user has
	// already navigated away from are dropped on the floor instead of
	// stamping stale state onto the new selection.
	itemClassifiedMsg struct {
		gen        int
		itemID     string
		isAssembly bool
		err        error
	}
)

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type breadcrumbEntry struct {
	id   string
	name string
}

// pendingNavState orchestrates the multi-step Show-in-Location flow.
// Folders are stored root → leaf; each contentsLoadedMsg consumes one
// hop until the slice is empty, then the cursor is placed on
// targetItemID inside the resulting folder.
type pendingNavState struct {
	targetItemID string
	folders      []api.FolderRef
}

// doubleClickWindow is how recent the previous click on the same row
// must have been to count as a double-click. 500 ms is the conventional
// threshold across most desktop environments.
const doubleClickWindow = 500 * time.Millisecond

// Model is the root bubbletea model for the fusionlocalserver browser.
type Model struct {
	state    appState
	width    int
	height   int
	err      error
	statusMsg string
	version   string

	// Auth
	token        string
	clientID     string
	clientSecret string

	// Hub data (shown as overlay, not a column)
	hubs       []api.NavItem
	hubCursor  int
	hubScroll  int
	hubLoading bool

	// Column data (projects=0, folders+items=1)
	cols    [numCols][]api.NavItem
	cursors [numCols]int
	loading [numCols]bool
	// scroll offsets for each column (for long lists)
	scrolls [numCols]int

	// Which column has keyboard focus
	activeCol int

	// Details panel (always visible)
	detailsLoading bool
	details        *api.ItemDetails
	detailsScroll  int
	// detailsCache memoises GetItemDetails results by item ID for the
	// lifetime of the session. Item details are immutable for a given ID
	// (a save creates a new version with a new tip-version number, but the
	// item ID is stable), so arrowing back over a previously-visited item
	// can be served synchronously without an API call. Refresh ([r]) and
	// hub re-selection clear the map to force a re-fetch.
	detailsCache map[string]*api.ItemDetails

	// Active tab in the details pane. Preserved when arrowing between
	// items in the same hub so a "scan Where Used across these designs"
	// flow works without re-pressing the tab key per item.
	detailsTab detailsTab
	// Per-tab loading flags (true while a fetch is in flight for the
	// currently-selected component version). Keyed by tab index.
	tabLoading [numDetailsTabs]bool
	// Per-tab last error message. Cleared on successful fetch or on
	// item change. Shown verbatim in the tab content area.
	tabErr [numDetailsTabs]string
	// Per-tab caches keyed by ComponentVersion.id. The cvid is whatever
	// m.details.RootComponentVersionID was when the fetch started, so
	// async responses arriving after a navigation are still stored under
	// the right key and don't pollute the active item's view.
	usesCache      map[string][]api.ComponentRef
	whereUsedCache map[string][]api.ComponentRef
	drawingsCache  map[string][]api.DrawingRef

	// Per-tab cursor index (only meaningful for tabUses / tabWhereUsed
	// / tabDrawings — tabDetails has its own scroll inside the details
	// view). Preserved across tab switches; reset to 0 when the
	// underlying selection changes.
	tabCursors [numDetailsTabs]int
	tabScrolls [numDetailsTabs]int

	// pendingNav drives the multi-step "Show in Location" navigation:
	// after a locate query returns, the project + folder chain is
	// queued here; subsequent contentsLoadedMsg handlers consume one
	// folder per drill until the chain is empty, then position the
	// cursor on targetItemID.
	pendingNav *pendingNavState

	// Mouse double-click tracking: a second click at the same logical
	// position within doubleClickWindow fires the activation action
	// (Show in Location for tab rows; reserved for future use elsewhere).
	lastClickKey string
	lastClickAt  time.Time

	// About / debug overlay scroll
	aboutScroll int
	debugScroll int

	// Pins overlay
	pins       []pins.Pin
	pinsCursor int
	pinsScroll int

	// contentsGen is incremented every time the Contents column's
	// underlying selection changes — folder drill, hub switch,
	// refresh, Show-in-Location, or any other path that swaps the
	// items slice. Async itemClassifiedMsg responses carry the gen
	// they were dispatched under and are dropped on mismatch, so a
	// classification answer that comes back after the user has
	// already moved on can't stamp stale "assembly" / "part" onto
	// the new selection.
	contentsGen int

	// For column 2: when drilling into a subfolder, track the stack so we can go back.
	folderStack []breadcrumbEntry

	// IDs of the currently selected hub and project. selectedHubName is
	// kept in sync with selectedHubID so we don't linear-scan m.hubs every
	// time we build a breadcrumb or status message.
	selectedHubID        string
	selectedHubAltID     string
	selectedHubNameCache string
	selectedProjectAltID string

	spinner      spinner.Model
	mouseEnabled bool

	// True while a STEP translation request + download is in flight. Used
	// to suppress concurrent [d] presses so multiple polls don't pile up.
	downloadInProgress bool

	// Render-time style cache. Lipgloss Styles are value types but their
	// rules clone on each chained call (.Width(...).Foreground(...)). The
	// browser View() runs at spinner rate (~10 Hz) and re-renders every
	// visible row each frame, so we precompute width-applied styles here
	// and rebuild only when the terminal size or theme changes. The cache
	// is shared by pointer because Bubble Tea passes the Model by value
	// to View(), so a local mutation on a copy would not persist.
	styleCache *styleCache
}

// styleCache holds Width-applied variants of the per-row styles used in
// renderColumn / viewDetailsColumn, plus the rendered detail-panel lines.
// navWidth and detailsInner are tracked so the cache is invalidated on
// resize; themeVersion bumps on cycleTheme.
type styleCache struct {
	navInner     int
	detailsInner int
	themeVersion int

	columnTitleNav     lipgloss.Style
	columnTitleDetails lipgloss.Style
	columnTitleHeading lipgloss.Style // styleColumnTitle with MarginBottom(0)
	itemSelectedNav    lipgloss.Style
	itemSelectedAccent lipgloss.Style // cursor-on-inactive variant
	itemNormalNav      lipgloss.Style
	containerItemNav   lipgloss.Style
	documentItemNav    lipgloss.Style
	pinnedItemNav      lipgloss.Style
	emptyNav           lipgloss.Style
	itemDimDetails     lipgloss.Style

	// Cached rendered detail lines for the current item. Keyed by the
	// pointer identity of m.details and the inner width; rebuild when
	// either changes (or on theme change, since rebuildStyleCache resets
	// the styles backing these strings).
	detailLinesPtr   *api.ItemDetails
	detailLinesWidth int
	detailLines      []string
}

// rebuild recomputes width-applied style variants. Called when the
// terminal is resized or the theme is cycled. Also drops the rendered
// detail-lines cache because it embeds styled strings produced from the
// previous theme/width.
func (sc *styleCache) rebuild(navInner, detailsInner int) {
	sc.navInner = navInner
	sc.detailsInner = detailsInner
	sc.themeVersion = themeVersion
	sc.columnTitleNav = styleColumnTitle.Width(navInner)
	sc.columnTitleDetails = styleColumnTitle.Width(detailsInner)
	sc.columnTitleHeading = styleColumnTitle.MarginBottom(0)
	sc.itemSelectedNav = styleItemSelected.Width(navInner)
	sc.itemSelectedAccent = styleItemNormal.Width(navInner).Foreground(colorAccent)
	sc.itemNormalNav = styleItemNormal.Width(navInner)
	sc.containerItemNav = styleContainerItem.Width(navInner)
	sc.documentItemNav = styleDocumentItem.Width(navInner)
	sc.pinnedItemNav = stylePinnedItem.Width(navInner)
	sc.emptyNav = styleEmpty.Width(navInner)
	sc.itemDimDetails = styleItemDim.Width(detailsInner)
	sc.detailLines = nil
	sc.detailLinesPtr = nil
	sc.detailLinesWidth = 0
}

// ensureStyleCache rebuilds the style cache if it's stale relative to the
// requested widths or current theme. The cache pointer is created in New()
// so all model copies share the same backing struct.
func (m Model) ensureStyleCache(navInner, detailsInner int) *styleCache {
	sc := m.styleCache
	if sc.navInner == navInner &&
		sc.detailsInner == detailsInner &&
		sc.themeVersion == themeVersion {
		return sc
	}
	sc.rebuild(navInner, detailsInner)
	return sc
}

// New creates the initial model. cfgErr may be non-nil when the config file is
// missing or invalid; the app will display a setup screen in that case.
func New(cfg *config.Config, cfgErr error, version string) Model {
	if os.Getenv("FUSIONLOCALSERVER_DEBUG") != "" {
		api.EnableDebug()
	}
	if cfg != nil && cfg.Region != "" {
		api.SetRegion(cfg.Region)
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleLoading

	// Promote any legacy single-file pins.json into per-hub files.
	// Idempotent (renames the legacy file on success). Pins are then
	// loaded lazily once a hub is selected — see the hubsLoadedMsg /
	// selectHub paths for the actual Load(hubID) call.
	_ = pins.MigrateLegacy()
	var loadedPins []pins.Pin

	if cfgErr != nil {
		return Model{
			state:          stateSetupNeeded,
			err:            cfgErr,
			spinner:        sp,
			version:        version,
			mouseEnabled:   true,
			pins:           loadedPins,
			detailsCache:   make(map[string]*api.ItemDetails),
			usesCache:      make(map[string][]api.ComponentRef),
			whereUsedCache: make(map[string][]api.ComponentRef),
			drawingsCache:  make(map[string][]api.DrawingRef),
			styleCache:     &styleCache{},
		}
	}

	return Model{
		state:          stateLoading,
		clientID:       cfg.ClientID,
		clientSecret:   cfg.ClientSecret,
		spinner:        sp,
		version:        version,
		mouseEnabled:   true,
		pins:           loadedPins,
		detailsCache:   make(map[string]*api.ItemDetails),
		usesCache:      make(map[string][]api.ComponentRef),
		whereUsedCache: make(map[string][]api.ComponentRef),
		drawingsCache:  make(map[string][]api.DrawingRef),
		styleCache:     &styleCache{},
	}
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func (m Model) Init() tea.Cmd {
	if m.state == stateSetupNeeded {
		return nil
	}
	return tea.Batch(m.spinner.Tick, checkTokensCmd(m.clientID, m.clientSecret))
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func checkTokensCmd(clientID, clientSecret string) tea.Cmd {
	return func() tea.Msg {
		td, err := auth.LoadTokens()
		if err != nil {
			return errMsg{err}
		}
		if td == nil {
			return tokenReadyMsg{token: ""}
		}
		if td.Valid() {
			return tokenReadyMsg{token: td.AccessToken}
		}
		if td.RefreshToken != "" {
			refreshed, err := auth.Refresh(context.Background(), clientID, clientSecret, td.RefreshToken)
			if err != nil {
				// Refresh failed — prompt fresh login.
				return tokenReadyMsg{token: ""}
			}
			return tokenReadyMsg{token: refreshed.AccessToken}
		}
		return tokenReadyMsg{token: ""}
	}
}

func loginCmd(clientID, clientSecret string) tea.Cmd {
	return func() tea.Msg {
		td, err := auth.Login(context.Background(), clientID, clientSecret)
		if err != nil {
			return errMsg{err}
		}
		return tokenReadyMsg{token: td.AccessToken}
	}
}

// navRequestTimeout bounds a single navigation GraphQL call. Generous enough
// to cover slow networks + multi-page pagination, short enough that a hung
// request doesn't leave the TUI frozen.
const navRequestTimeout = 30 * time.Second

func loadHubsCmd(token string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), navRequestTimeout)
		defer cancel()
		items, err := api.GetHubs(ctx, token)
		if err != nil {
			return errMsg{err}
		}
		return hubsLoadedMsg{items}
	}
}

func loadProjectsCmd(token, hubID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), navRequestTimeout)
		defer cancel()
		items, err := api.GetProjects(ctx, token, hubID)
		if err != nil {
			return errMsg{err}
		}
		return projectsLoadedMsg{items}
	}
}

// loadProjectContentsCmd loads the root contents of a project.
// It fetches both top-level folders (foldersByProject) and project-level items
// (itemsByProject) concurrently and merges them — folders first, then items.
func loadProjectContentsCmd(token, projectID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), navRequestTimeout)
		defer cancel()

		var (
			folders, items []api.NavItem
			fErr, iErr     error
			wg             sync.WaitGroup
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			folders, fErr = api.GetFolders(ctx, token, projectID)
		}()
		go func() {
			defer wg.Done()
			items, iErr = api.GetProjectItems(ctx, token, projectID)
		}()
		wg.Wait()
		if fErr != nil {
			return errMsg{fErr}
		}
		if iErr != nil {
			return errMsg{iErr}
		}
		combined := append(folders, items...)
		return contentsLoadedMsg{combined}
	}
}

func loadItemsCmd(token, hubID, folderID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), navRequestTimeout)
		defer cancel()
		items, err := api.GetItems(ctx, token, hubID, folderID)
		if err != nil {
			return errMsg{err}
		}
		return contentsLoadedMsg{items}
	}
}

// classifyDesignCmd runs api.ClassifyAssembly off the main goroutine and
// posts an itemClassifiedMsg back to Update when it completes. The gen
// argument is captured at dispatch time and echoed in the response so a
// late refinement after the user has navigated away can be ignored — see
// the itemClassifiedMsg handler in Update for the gen check. The API
// call itself rate-limits via a package-level semaphore (8 concurrent),
// so a fanout of 50 cmds here doesn't translate into 50 simultaneous
// HTTPS connections against the gateway.
func classifyDesignCmd(token, itemID, componentVersionID string, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), navRequestTimeout)
		defer cancel()
		isAsm, err := api.ClassifyAssembly(ctx, token, componentVersionID)
		return itemClassifiedMsg{gen: gen, itemID: itemID, isAssembly: isAsm, err: err}
	}
}

// classifyContentsCmd fans out classifyDesignCmd over every DesignItem
// in items that has a known componentVersionID. Returns nil when nothing
// needs classifying so Update can return (m, nil) cleanly. The cmds run
// concurrently under tea.Batch; the gateway-side concurrency cap lives
// in api.ClassifyAssembly's semaphore.
func classifyContentsCmd(token string, items []api.NavItem, gen int) tea.Cmd {
	var cmds []tea.Cmd
	for _, it := range items {
		if it.Kind == "design" && it.ComponentVersionID != "" && it.Subtype == "" {
			cmds = append(cmds, classifyDesignCmd(token, it.ID, it.ComponentVersionID, gen))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func loadDetailsCmd(token, hubID, itemID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), navRequestTimeout)
		defer cancel()
		d, err := api.GetItemDetails(ctx, token, hubID, itemID)
		if err != nil {
			return errMsg{err}
		}
		return detailsLoadedMsg{d}
	}
}

// Per-tab fetch commands. cvid is captured into the message so a stale
// response that arrives after the user navigated to a different item is
// still cached under the right ComponentVersion id.

func loadUsesCmd(token, cvid string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), navRequestTimeout)
		defer cancel()
		items, err := api.GetOccurrences(ctx, token, cvid)
		return usesLoadedMsg{cvid: cvid, items: items, err: err}
	}
}

// loadDrawingUsesCmd fetches the source design for a drawing item.
// "Uses" on a drawing is the design the drawing was made from. The
// resulting message reuses usesLoadedMsg so the receiver doesn't need
// to know which underlying API was called — the cvid field carries
// the drawing item's id (acting as the cache key for this code path).
func loadDrawingUsesCmd(token, hubID, drawingItemID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), navRequestTimeout)
		defer cancel()
		items, err := api.GetDrawingSource(ctx, token, hubID, drawingItemID)
		return usesLoadedMsg{cvid: drawingItemID, items: items, err: err}
	}
}

func loadWhereUsedCmd(token, cvid string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), navRequestTimeout)
		defer cancel()
		items, err := api.GetWhereUsed(ctx, token, cvid)
		return whereUsedLoadedMsg{cvid: cvid, items: items, err: err}
	}
}

func loadDrawingsCmd(token, hubID, itemID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), navRequestTimeout)
		defer cancel()
		items, err := api.GetDrawingsForDesign(ctx, token, hubID, itemID)
		return drawingsLoadedMsg{itemID: itemID, items: items, err: err}
	}
}

// locateItemCmd resolves an item's project + folder ancestry so the
// Update loop can orchestrate the Show-in-Location navigation.
func locateItemCmd(token, hubID, itemID string) tea.Cmd {
	return func() tea.Msg {
		// Locate may fan out into many folder-walk requests; allow a
		// generous timeout above the per-call nav budget.
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		loc, err := api.GetItemLocation(ctx, token, hubID, itemID)
		return itemLocationLoadedMsg{loc: loc, target: itemID, err: err}
	}
}

func openURLCmd(u string) tea.Cmd {
	return func() tea.Msg {
		api.DebugLog("OPEN_BROWSER %s", u)
		_ = auth.OpenBrowser(u)
		return openedBrowserMsg{url: u}
	}
}

// openInFusionCmd asks the running Fusion desktop app (via its local MCP
// server) to open the document identified by the lineage URN. Before sending
// the open call, it verifies that Fusion's active hub contains the CLI's
// currently-selected project; if not, it returns a message instructing the
// user to switch hubs in Fusion and performs no action.
func openInFusionCmd(fileID, expectedProjectAltID, expectedProjectName, expectedHubName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		client := fusion.NewClient()
		if err := verifySameHub(ctx, client, expectedProjectAltID, expectedProjectName, expectedHubName); err != nil {
			return fusionActionMsg{err: err}
		}
		if err := client.OpenDocument(ctx, fileID); err != nil {
			return fusionActionMsg{err: err}
		}
		return fusionActionMsg{action: "Opened in Fusion"}
	}
}

// insertInFusionCmd asks the running Fusion desktop app (via its local MCP
// server) to insert the document identified by the lineage URN as an
// occurrence in the active design. Blocked if Fusion is on a different hub
// (see openInFusionCmd).
func insertInFusionCmd(fileID, expectedProjectAltID, expectedProjectName, expectedHubName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		client := fusion.NewClient()
		if err := verifySameHub(ctx, client, expectedProjectAltID, expectedProjectName, expectedHubName); err != nil {
			return fusionActionMsg{err: err}
		}
		if err := client.InsertDocument(ctx, fileID); err != nil {
			return fusionActionMsg{err: err}
		}
		return fusionActionMsg{action: "Inserted in Fusion"}
	}
}

// requestStepCmd issues the GraphQL query that initiates STEP generation
// (the first call) and reports its current status (subsequent calls). The
// query is idempotent: APS keeps generating the derivative between calls.
func requestStepCmd(token, cvid, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		status, signedURL, err := api.RequestSTEPDerivative(ctx, token, cvid)
		return stepStatusMsg{
			status:    status,
			signedURL: signedURL,
			err:       err,
			cvid:      cvid,
			name:      name,
		}
	}
}

// pollStepCmdAfter waits d, then re-issues the STEP status query.
func pollStepCmdAfter(d time.Duration, token, cvid, name string) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		status, signedURL, err := api.RequestSTEPDerivative(ctx, token, cvid)
		return stepStatusMsg{
			status:    status,
			signedURL: signedURL,
			err:       err,
			cvid:      cvid,
			name:      name,
		}
	})
}

// downloadStepFileCmd streams the signed-URL response to a file under the
// user's Downloads directory and returns a stepDoneMsg with the final path
// (or any error encountered).
func downloadStepFileCmd(signedURL, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		path := api.StepDownloadPath(name)
		if err := api.DownloadFile(ctx, signedURL, path); err != nil {
			return stepDoneMsg{err: err}
		}
		return stepDoneMsg{path: path}
	}
}

// verifySameHub returns nil when Fusion's active hub contains the CLI's
// currently-selected project. Otherwise it returns an error whose message
// names the expected hub so the status bar can tell the user to switch
// hubs in Fusion.
//
// The CLI stores a project's APS Data Management API ID (e.g.
// "a.YnVzaW5lc3M6YXV0b2Rlc2s4MDgzIzIwMjUwMjEzODc2NjAyNTMx") but Fusion's
// local MCP server reports the raw internal ID (e.g. "20250213876602531"),
// so we convert with fusion.NormalizeProjectID before comparing.
//
// An empty expectedProjectAltID (e.g. if the CLI hasn't drilled into a
// project yet) skips the check. If conversion fails, we fall back to a
// case-insensitive match on the project name so we don't spuriously block
// on an unexpected ID format.
func verifySameHub(ctx context.Context, client *fusion.Client, expectedProjectAltID, expectedProjectName, expectedHubName string) error {
	if expectedProjectAltID == "" && expectedProjectName == "" {
		return nil
	}
	projects, err := client.ActiveHubProjects(ctx)
	if err != nil {
		return fmt.Errorf("could not verify Fusion hub: %w", err)
	}
	wantID := fusion.NormalizeProjectID(expectedProjectAltID)
	wantName := strings.TrimSpace(strings.ToLower(expectedProjectName))
	for _, p := range projects {
		if wantID != "" && p.ID == wantID {
			return nil
		}
		if wantID == "" && wantName != "" && strings.TrimSpace(strings.ToLower(p.Name)) == wantName {
			return nil
		}
	}
	hubLabel := expectedHubName
	if hubLabel == "" {
		hubLabel = "the selected hub"
	}
	return fmt.Errorf("Fusion is on a different hub — switch Fusion to %q and retry", hubLabel)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tokenReadyMsg:
		if msg.token == "" {
			m.state = stateAuthNeeded
			return m, nil
		}
		m.token = msg.token
		m.state = stateLoading
		m.hubLoading = true
		return m, loadHubsCmd(m.token)

	case hubsLoadedMsg:
		m.hubLoading = false
		m.hubs = msg.items
		m.hubCursor = 0
		m.hubScroll = 0
		// Auto-select if only one hub, otherwise show hub overlay
		if len(msg.items) == 1 {
			m.state = stateBrowsing
			m.activeCol = colProjects
			m.selectedHubID = msg.items[0].ID
			m.selectedHubAltID = msg.items[0].AltID
			m.selectedHubNameCache = msg.items[0].Name
			m.loading[colProjects] = true
			m.loadPinsForCurrentHub()
			return m, loadProjectsCmd(m.token, msg.items[0].ID)
		}
		m.state = stateHubSelect
		m.activeCol = colProjects
		return m, nil

	case projectsLoadedMsg:
		m.loading[colProjects] = false
		m.cols[colProjects] = msg.items
		m.cursors[colProjects] = 0
		m.scrolls[colProjects] = 0
		// Clear stale contents and bump the contents generation so any
		// classification cmds in flight for the previous project's
		// items don't paint state onto an empty / different selection
		// when they return.
		m.cols[colContents] = nil
		m.contentsGen++
		m.folderStack = nil
		m.selectedProjectAltID = ""
		return m, nil

	case itemClassifiedMsg:
		// Late refinements that belong to a folder the user has
		// already navigated away from are silently dropped — the gen
		// check is the canonical cancellation mechanism. We also
		// swallow per-row errors here: a failed classification just
		// means the row keeps its generic design icon, which is the
		// graceful-degradation we want.
		if msg.gen != m.contentsGen || msg.err != nil {
			return m, nil
		}
		for i, it := range m.cols[colContents] {
			if it.ID == msg.itemID {
				if msg.isAssembly {
					m.cols[colContents][i].Subtype = "assembly"
				} else {
					m.cols[colContents][i].Subtype = "part"
				}
				break
			}
		}
		return m, nil

	case contentsLoadedMsg:
		m.loading[colContents] = false
		m.cols[colContents] = msg.items
		m.cursors[colContents] = 0
		m.scrolls[colContents] = 0
		// Bump the contents generation so any in-flight classifier
		// responses for the previous folder fail their gen check and
		// get dropped — see itemClassifiedMsg handler.
		m.contentsGen++

		// Show-in-Location orchestration. When pendingNav is set we are
		// in a multi-step nav: each contentsLoadedMsg either drills the
		// next folder in the queue or — when the queue is empty —
		// positions the cursor on the target item.
		if m.pendingNav != nil {
			if len(m.pendingNav.folders) > 0 {
				target := m.pendingNav.folders[0]
				m.pendingNav.folders = m.pendingNav.folders[1:]
				for i, it := range msg.items {
					if it.ID == target.ID && it.IsContainer {
						m.cursors[colContents] = i
						m.adjustScroll(colContents)
						m.folderStack = append(m.folderStack, breadcrumbEntry{id: target.ID, name: target.Name})
						m.cols[colContents] = nil
						m.loading[colContents] = true
						// Skip classification on intermediate hops:
						// these items are about to be replaced by the
						// next folder's contents anyway.
						return m, loadItemsCmd(m.token, m.selectedHubID, target.ID)
					}
				}
				// Folder chain broken (could happen if APS returned a
				// folder ID that isn't actually visible to this user) —
				// surface the failure and fall through to the
				// auto-details path so we still render *something*.
				m.statusMsg = "Folder not found in contents: " + target.Name
				m.pendingNav = nil
			} else {
				for i, it := range msg.items {
					if it.ID == m.pendingNav.targetItemID {
						m.cursors[colContents] = i
						m.adjustScroll(colContents)
						break
					}
				}
				m.pendingNav = nil
			}
		}

		// Fire assembly-vs-part classification for every DesignItem in
		// the freshly-loaded items. Cmds run concurrently under
		// tea.Batch; api.ClassifyAssembly's package-level semaphore
		// caps real parallelism so we don't stampede the gateway.
		classifyCmd := classifyContentsCmd(m.token, msg.items, m.contentsGen)

		// Auto-load details for whatever item the cursor now points at.
		// Without pendingNav this is index 0 (the original behavior);
		// with pendingNav it's the located target.
		cur := m.cursors[colContents]
		if cur < len(msg.items) && !msg.items[cur].IsContainer && m.selectedHubID != "" {
			m.detailsScroll = 0
			m.resetTabState()
			if cached, ok := m.detailsCache[msg.items[cur].ID]; ok && cached != nil {
				m.details = cached
				m.detailsLoading = false
				return m, tea.Batch(m.maybeLoadActiveTab(), classifyCmd)
			}
			m.detailsLoading = true
			m.details = nil
			return m, tea.Batch(loadDetailsCmd(m.token, m.selectedHubID, msg.items[cur].ID), classifyCmd)
		}
		m.details = nil
		return m, classifyCmd

	case itemLocationLoadedMsg:
		return m.handleItemLocation(msg)

	case detailsLoadedMsg:
		m.detailsLoading = false
		m.details = msg.details
		m.detailsScroll = 0
		if msg.details != nil && msg.details.ID != "" {
			m.detailsCache[msg.details.ID] = msg.details
		}
		// Reset per-tab error/loading flags — they applied to the
		// previous selection — and kick off a fetch for the active
		// tab if it isn't Details and the new cvid isn't cached.
		m.resetTabState()
		return m, m.maybeLoadActiveTab()

	case usesLoadedMsg:
		if msg.err == nil && m.usesCache != nil {
			m.usesCache[msg.cvid] = msg.items
		}
		// usesCacheKey() returns whatever string the active item uses
		// to key its Uses entry — DesignItem.RootComponentVersionID
		// for designs, DesignItem.ID for drawings. The response's
		// msg.cvid carries the same string the load command sent, so
		// matching here confirms the response is for the current item.
		if m.details != nil && m.usesCacheKey() == msg.cvid {
			m.tabLoading[tabUses] = false
			if msg.err != nil {
				m.tabErr[tabUses] = msg.err.Error()
			} else {
				m.tabErr[tabUses] = ""
			}
		}
		return m, nil

	case whereUsedLoadedMsg:
		if msg.err == nil && m.whereUsedCache != nil {
			m.whereUsedCache[msg.cvid] = msg.items
		}
		if m.details != nil && m.details.RootComponentVersionID == msg.cvid {
			m.tabLoading[tabWhereUsed] = false
			if msg.err != nil {
				m.tabErr[tabWhereUsed] = msg.err.Error()
			} else {
				m.tabErr[tabWhereUsed] = ""
			}
		}
		return m, nil

	case drawingsLoadedMsg:
		if msg.err == nil && m.drawingsCache != nil {
			m.drawingsCache[msg.itemID] = msg.items
		}
		if m.details != nil && m.details.ID == msg.itemID {
			m.tabLoading[tabDrawings] = false
			if msg.err != nil {
				m.tabErr[tabDrawings] = msg.err.Error()
			} else {
				m.tabErr[tabDrawings] = ""
			}
		}
		return m, nil

	case openedBrowserMsg:
		// Show the URL in the status bar so users can see exactly what
		// was opened. If the browser page errors (e.g. Autodesk returns
		// "WEB SESSION INVALID") the user can then confirm the URL, sign
		// in to accounts.autodesk.com in their browser, and retry.
		if msg.url != "" {
			m.statusMsg = "Opened: " + msg.url
		} else {
			m.statusMsg = "Opened in browser"
		}
		return m, nil

	case fusionActionMsg:
		if msg.err != nil {
			m.statusMsg = "Fusion: " + msg.err.Error()
		} else {
			m.statusMsg = msg.action
		}
		return m, nil

	case stepStatusMsg:
		if msg.err != nil {
			m.downloadInProgress = false
			m.statusMsg = "STEP error: " + msg.err.Error()
			return m, nil
		}
		switch msg.status {
		case api.StepStatusSuccess:
			m.statusMsg = "Downloading STEP for " + msg.name + "…"
			return m, downloadStepFileCmd(msg.signedURL, msg.name)
		case api.StepStatusFailed:
			m.downloadInProgress = false
			m.statusMsg = "STEP translation failed for " + msg.name
			return m, nil
		default:
			// PENDING (or any other transient state) — keep polling.
			m.statusMsg = "Generating STEP for " + msg.name + "… (this may take a moment)"
			return m, pollStepCmdAfter(2*time.Second, m.token, msg.cvid, msg.name)
		}

	case stepDoneMsg:
		m.downloadInProgress = false
		if msg.err != nil {
			m.statusMsg = "STEP download failed: " + msg.err.Error()
			return m, nil
		}
		m.statusMsg = "Saved STEP file: " + msg.path
		return m, nil

	case errMsg:
		m.err = msg.err
		m.state = stateError
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.About):
		if m.state == stateAbout {
			m.state = stateBrowsing
		} else if m.state == stateBrowsing || m.state == stateAuthNeeded {
			m.aboutScroll = 0
			m.state = stateAbout
		}
		return m, nil

	case m.state == stateAbout && key.Matches(msg, keys.Up):
		if m.aboutScroll > 0 {
			m.aboutScroll--
		}
		return m, nil

	case m.state == stateAbout && key.Matches(msg, keys.Down):
		m.aboutScroll++
		return m, nil

	case m.state == stateAbout:
		// any other key closes about
		m.state = stateBrowsing
		return m, nil

	case key.Matches(msg, keys.Hub):
		if m.state == stateHubSelect {
			m.state = stateBrowsing
		} else if m.state == stateBrowsing {
			m.hubScroll = 0
			m.state = stateHubSelect
		}
		return m, nil

	case m.state == stateHubSelect && key.Matches(msg, keys.Up):
		if len(m.hubs) > 0 && m.hubCursor > 0 {
			m.hubCursor--
			m.adjustHubScroll()
		}
		return m, nil

	case m.state == stateHubSelect && key.Matches(msg, keys.Down):
		if len(m.hubs) > 0 && m.hubCursor < len(m.hubs)-1 {
			m.hubCursor++
			m.adjustHubScroll()
		}
		return m, nil

	case m.state == stateHubSelect && (key.Matches(msg, keys.Enter) || key.Matches(msg, keys.Right)):
		return m.selectHub()

	case m.state == stateHubSelect && key.Matches(msg, keys.Refresh):
		m.hubs = nil
		m.hubLoading = true
		return m, loadHubsCmd(m.token)

	case m.state == stateHubSelect:
		// ignore other keys in hub overlay
		return m, nil

	case key.Matches(msg, keys.Debug):
		if m.state == stateDebug {
			m.state = stateBrowsing
		} else if m.state == stateBrowsing {
			m.debugScroll = 0
			m.state = stateDebug
		}
		return m, nil

	case m.state == stateDebug && key.Matches(msg, keys.Up):
		if m.debugScroll > 0 {
			m.debugScroll--
		}
		return m, nil

	case m.state == stateDebug && key.Matches(msg, keys.Down):
		m.debugScroll++
		return m, nil

	case key.Matches(msg, keys.PinsOpen):
		if m.state == statePins {
			m.state = stateBrowsing
		} else if m.state == stateBrowsing {
			m.pinsCursor = 0
			m.pinsScroll = 0
			m.state = statePins
		}
		return m, nil

	case m.state == statePins && key.Matches(msg, keys.Up):
		if m.pinsCursor > 0 {
			m.pinsCursor--
			m.adjustPinsScroll()
		}
		return m, nil

	case m.state == statePins && key.Matches(msg, keys.Down):
		if m.pinsCursor < len(m.pins)-1 {
			m.pinsCursor++
			m.adjustPinsScroll()
		}
		return m, nil

	case m.state == statePins && key.Matches(msg, keys.Enter):
		return m.navigateToPinnedItem()

	case m.state == statePins && key.Matches(msg, keys.PinDelete):
		return m.removePinnedItem()

	case m.state == statePins && key.Matches(msg, keys.OpenDesktop):
		return m.openPinnedInDesktop()

	case m.state == statePins && key.Matches(msg, keys.Insert):
		return m.insertPinnedInDesktop()

	case m.state == statePins:
		// any other key closes the pins overlay
		m.state = stateBrowsing
		return m, nil

	case m.state == stateAuthNeeded && key.Matches(msg, keys.Enter):
		m.state = stateAuthWaiting
		return m, tea.Batch(m.spinner.Tick, loginCmd(m.clientID, m.clientSecret))

	case m.state == stateError && (key.Matches(msg, keys.Refresh) || key.Matches(msg, keys.Enter)):
		return m.recoverFromError()

	case m.state != stateBrowsing:
		return m, nil

	case key.Matches(msg, keys.Up):
		// On a non-Details tab, ↑/↓ drive the tab's row cursor instead
		// of the Contents column. Switch back to Details (key 1) to
		// resume normal nav.
		if m.tabsAvailable() && m.detailsTab != tabDetails {
			m.moveTabCursor(-1)
			return m, nil
		}
		m.moveCursor(-1)
		m.detailsScroll = 0
		return m, m.maybeLoadDetails()

	case key.Matches(msg, keys.Down):
		if m.tabsAvailable() && m.detailsTab != tabDetails {
			m.moveTabCursor(1)
			return m, nil
		}
		m.moveCursor(1)
		m.detailsScroll = 0
		return m, m.maybeLoadDetails()

	case key.Matches(msg, keys.Left):
		return m.navigateLeft()

	case key.Matches(msg, keys.Enter):
		// Enter activates: on a non-Details tab → Show in Location for
		// the highlighted row; otherwise the existing nav-right
		// behavior (drill into the selected folder/project).
		if m.tabsAvailable() && m.detailsTab != tabDetails {
			return m.showInLocation()
		}
		return m.navigateRight()

	case key.Matches(msg, keys.Right):
		return m.navigateRight()

	case key.Matches(msg, keys.Open):
		return m.openInBrowser()

	case key.Matches(msg, keys.OpenDesktop):
		return m.openInDesktop()

	case key.Matches(msg, keys.Insert):
		return m.insertInDesktop()

	case key.Matches(msg, keys.Download):
		return m.downloadStep()

	case key.Matches(msg, keys.PinToggle):
		return m.togglePin()

	case key.Matches(msg, keys.Refresh):
		return m.refresh()

	case key.Matches(msg, keys.Theme):
		name := cycleTheme()
		m.spinner.Style = styleLoading
		m.statusMsg = "Theme: " + name
		return m, nil

	case key.Matches(msg, keys.Mouse):
		m.mouseEnabled = !m.mouseEnabled
		if m.mouseEnabled {
			m.statusMsg = "Mouse: on"
			return m, tea.EnableMouseCellMotion
		}
		m.statusMsg = "Mouse: off"
		return m, tea.DisableMouse

	case key.Matches(msg, keys.TabSelect):
		switch msg.String() {
		case "1":
			return m.selectTab(tabDetails)
		case "2":
			return m.selectTab(tabUses)
		case "3":
			return m.selectTab(tabWhereUsed)
		case "4":
			return m.selectTab(tabDrawings)
		}
		return m, nil

	}

	return m, nil
}

// handleMouse processes mouse events when mouse support is enabled.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.mouseEnabled {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.mouseScroll(-3)
	case tea.MouseButtonWheelDown:
		return m.mouseScroll(3)
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		return m.mouseClick(msg.X, msg.Y)
	}
	return m, nil
}

// mouseScroll handles scroll wheel events based on current state.
func (m Model) mouseScroll(delta int) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateBrowsing:
		m.moveCursor(delta)
		m.detailsScroll = 0
		return m, m.maybeLoadDetails()
	case stateHubSelect:
		for range abs(delta) {
			if delta < 0 && m.hubCursor > 0 {
				m.hubCursor--
			} else if delta > 0 && m.hubCursor < len(m.hubs)-1 {
				m.hubCursor++
			}
		}
		m.adjustHubScroll()
		return m, nil
	case stateAbout:
		m.aboutScroll += delta
		if m.aboutScroll < 0 {
			m.aboutScroll = 0
		}
		return m, nil
	case stateDebug:
		m.debugScroll += delta
		if m.debugScroll < 0 {
			m.debugScroll = 0
		}
		return m, nil
	}
	return m, nil
}

// mouseClick handles left-click events in the browsing state.
func (m Model) mouseClick(x, y int) (tea.Model, tea.Cmd) {
	if m.state == stateHubSelect {
		return m.mouseClickHub(y)
	}
	if m.state != stateBrowsing {
		return m, nil
	}

	// Breadcrumb hit test: the header is on row 0. If the click lands on a
	// clickable segment, jump to that level instead of falling through to
	// the column-click logic.
	if y == 0 {
		if _, hits := m.buildBreadcrumb(breadcrumbXOffset()); len(hits) > 0 {
			for _, h := range hits {
				if x >= h.xStart && x < h.xEnd {
					return m.clickBreadcrumb(h)
				}
			}
		}
		return m, nil
	}

	// Determine column layout (mirrors viewBrowser).
	detailsWidth := (m.width * 35) / 100
	navWidth := m.width - detailsWidth - 2
	colWidth := (navWidth - 4) / numCols
	if colWidth < 10 {
		colWidth = 10
	}

	// Y layout: header(1) + border-top(1) + title-row(1) + padding = first item at y=4.
	const firstItemY = 4

	// Details column starts after the two nav columns. Clicks landing in
	// the details area only matter when a non-Details tab is active —
	// they let the user single-click to highlight and double-click to
	// trigger Show in Location.
	detailsStart := numCols * colWidth
	if x >= detailsStart && m.tabsAvailable() && m.detailsTab != tabDetails {
		row := y - firstItemY
		if row < 0 {
			return m, nil
		}
		// Approximate ref index from line offset: refs render to 1-2
		// lines each. Half-line resolution is acceptable for click
		// targeting and matches the scroll heuristic in moveTabCursor.
		approxRow := m.tabScrolls[m.detailsTab] + row/2
		rows := m.currentTabRowCount()
		if approxRow < 0 || approxRow >= rows {
			return m, nil
		}
		clickKey := fmt.Sprintf("tab%d:%d", m.detailsTab, approxRow)
		now := time.Now()
		isDouble := m.lastClickKey == clickKey && now.Sub(m.lastClickAt) < doubleClickWindow
		m.lastClickKey = clickKey
		m.lastClickAt = now
		m.tabCursors[m.detailsTab] = approxRow
		if isDouble {
			return m.showInLocation()
		}
		return m, nil
	}

	// Each column is rendered with style.Width(colWidth) which is the outer
	// width (includes border + padding). Columns are placed side-by-side by
	// lipgloss.JoinHorizontal.
	col := -1
	for i := 0; i < numCols; i++ {
		colStart := i * colWidth
		colEnd := colStart + colWidth
		if x >= colStart && x < colEnd {
			col = i
			break
		}
	}

	row := y - firstItemY

	if col < 0 {
		return m, nil
	}

	row += m.scrolls[col]
	items := m.cols[col]
	if row < 0 || row >= len(items) {
		return m, nil
	}

	if col != m.activeCol {
		m.activeCol = col
	}

	m.cursors[col] = row
	m.adjustScroll(col)
	m.detailsScroll = 0
	m.lastClickKey = "" // any nav-column click breaks the double-click chain

	// For projects column or folders in contents, navigate into the item.
	// For documents in contents, just load details.
	item := m.cols[col][row]
	if col == colProjects || item.IsContainer {
		return m.navigateRight()
	}
	return m, m.maybeLoadDetails()
}

// mouseClickHub handles clicking on a hub in the hub selection overlay.
func (m Model) mouseClickHub(y int) (tea.Model, tea.Cmd) {
	// The hub overlay is centered; rows start after the overlay border + title.
	// Approximate: the overlay header takes ~3 rows from top of overlay.
	// Since exact positioning depends on centering, use a simpler approach:
	// map y to hub index relative to scroll.
	const overlayHeaderRows = 4 // border + title + blank + list start
	centerY := (m.height - len(m.hubs) - overlayHeaderRows) / 2
	if centerY < 0 {
		centerY = 0
	}
	idx := y - centerY - overlayHeaderRows + m.hubScroll
	if idx < 0 || idx >= len(m.hubs) {
		return m, nil
	}
	m.hubCursor = idx
	return m.selectHub()
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// moveTabCursor moves the active non-Details tab's row cursor by delta,
// clamped to the tab's row count, and adjusts the per-tab scroll
// offset so the cursor stays visible.
func (m *Model) moveTabCursor(delta int) {
	rows := m.currentTabRowCount()
	if rows == 0 {
		m.tabCursors[m.detailsTab] = 0
		m.tabScrolls[m.detailsTab] = 0
		return
	}
	cur := clamp(m.tabCursors[m.detailsTab]+delta, 0, rows-1)
	m.tabCursors[m.detailsTab] = cur
	// Visible rows in the details pane is roughly height - fixed
	// chrome (header + footer + borders + tab strip + bottom hints).
	// Use a conservative estimate so the cursor doesn't overshoot.
	visible := m.height - 9
	if visible < 1 {
		visible = 1
	}
	scroll := m.tabScrolls[m.detailsTab]
	if cur < scroll {
		m.tabScrolls[m.detailsTab] = cur
	} else if cur >= scroll+visible {
		m.tabScrolls[m.detailsTab] = cur - visible + 1
	}
}

// moveCursor moves the cursor in the active column and adjusts scroll.
func (m *Model) moveCursor(delta int) {
	col := m.activeCol
	items := m.cols[col]
	if len(items) == 0 {
		return
	}
	m.cursors[col] = clamp(m.cursors[col]+delta, 0, len(items)-1)
	m.adjustScroll(col)
}

// adjustScroll keeps the cursor visible in the column.
func (m *Model) adjustScroll(col int) {
	visible := m.visibleRows()
	c := m.cursors[col]
	s := m.scrolls[col]
	if c < s {
		m.scrolls[col] = c
	} else if c >= s+visible {
		m.scrolls[col] = c - visible + 1
	}
}

// crumbHit describes one clickable segment of the breadcrumb bar.
// xStart is inclusive, xEnd is exclusive, both measured in terminal columns
// from the leftmost column of the window.
type crumbHit struct {
	xStart, xEnd int
	kind         string // "hub" | "project" | "folder"
	index        int    // folder stack index for "folder", unused otherwise
}

// breadcrumbXOffset returns the absolute x column at which the breadcrumb
// segments begin inside the header row. It accounts for the left padding of
// styleHeader plus the fixed "fusionlocalserver  " prefix.
func breadcrumbXOffset() int {
	// styleHeader.Padding(0, 1) contributes 1 leading column.
	return 1 + lipgloss.Width("fusionlocalserver  ")
}

// buildBreadcrumb returns the plain-text breadcrumb string (with " › "
// separators) and the list of clickable segment regions. xOffset is the
// absolute x column of the first rune of the breadcrumb text.
//
// The terminal document (a non-container item on colContents) is included in
// the text but is NOT clickable — clicking the currently shown document does
// nothing useful beyond what's already on screen.
func (m Model) buildBreadcrumb(xOffset int) (string, []crumbHit) {
	const sep = " › "
	sepW := lipgloss.Width(sep)

	var sb strings.Builder
	var hits []crumbHit
	x := xOffset
	first := true

	addSeg := func(text, kind string, idx int, clickable bool) {
		if text == "" {
			return
		}
		if !first {
			sb.WriteString(sep)
			x += sepW
		}
		first = false
		w := lipgloss.Width(text)
		if clickable {
			hits = append(hits, crumbHit{xStart: x, xEnd: x + w, kind: kind, index: idx})
		}
		sb.WriteString(text)
		x += w
	}

	if m.selectedHubNameCache != "" {
		addSeg(m.selectedHubNameCache, "hub", 0, true)
	}
	if proj := m.selectedItem(colProjects); proj != nil {
		addSeg(proj.Name, "project", 0, true)
	}
	for i, f := range m.folderStack {
		addSeg(f.name, "folder", i, true)
	}
	if item := m.selectedItem(colContents); item != nil && !item.IsContainer {
		addSeg(item.Name, "document", 0, false)
	}
	return sb.String(), hits
}

// clickBreadcrumb navigates to the level described by a breadcrumb hit.
//
//   - hub:     opens the hub-select overlay.
//   - project: clears the folder stack and reloads the project's root.
//   - folder:  truncates the folder stack to the clicked depth and reloads
//     the contents of that folder.
func (m Model) clickBreadcrumb(h crumbHit) (Model, tea.Cmd) {
	switch h.kind {
	case "hub":
		m.hubScroll = 0
		m.state = stateHubSelect
		return m, nil

	case "project":
		proj := m.selectedItem(colProjects)
		if proj == nil {
			return m, nil
		}
		m.selectedProjectAltID = proj.AltID
		m.activeCol = colContents
		m.folderStack = nil
		m.cols[colContents] = nil
		m.loading[colContents] = true
		m.details = nil
		m.detailsScroll = 0
		return m, loadProjectContentsCmd(m.token, proj.ID)

	case "folder":
		if h.index < 0 || h.index >= len(m.folderStack) {
			return m, nil
		}
		// Truncate to include only up to and including the clicked folder.
		target := m.folderStack[h.index]
		m.folderStack = m.folderStack[:h.index+1]
		m.activeCol = colContents
		m.cols[colContents] = nil
		m.loading[colContents] = true
		m.details = nil
		m.detailsScroll = 0
		return m, loadItemsCmd(m.token, m.selectedHubID, target.id)
	}
	return m, nil
}

// navigateLeft moves focus left or goes up a folder level, returning a reload
// command when the folder stack is popped.
func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.activeCol {
	case colContents:
		if len(m.folderStack) > 0 {
			// Pop folder stack and reload the parent's contents.
			m.folderStack = m.folderStack[:len(m.folderStack)-1]
			m.cols[colContents] = nil
			m.loading[colContents] = true
			if len(m.folderStack) > 0 {
				// Reload the folder that's now on top of the stack.
				return m, loadItemsCmd(m.token, m.selectedHubID, m.folderStack[len(m.folderStack)-1].id)
			}
			// Back to project root folders.
			proj := m.selectedItem(colProjects)
			if proj != nil {
				return m, loadProjectContentsCmd(m.token, proj.ID)
			}
			m.loading[colContents] = false
		} else {
			m.activeCol = colProjects
		}
	case colProjects:
		// Already at leftmost column.
	}
	return m, nil
}

// navigateRight moves focus right, loading the next level.
func (m Model) navigateRight() (Model, tea.Cmd) {
	switch m.activeCol {
	case colProjects:
		item := m.selectedItem(colProjects)
		if item == nil {
			return m, nil
		}
		m.selectedProjectAltID = item.AltID
		m.activeCol = colContents
		m.cols[colContents] = nil
		m.folderStack = nil
		m.loading[colContents] = true
		return m, loadProjectContentsCmd(m.token, item.ID)

	case colContents:
		item := m.selectedItem(colContents)
		if item == nil {
			return m, nil
		}
		if item.IsContainer {
			// Drill into sub-folder.
			m.folderStack = append(m.folderStack, breadcrumbEntry{id: item.ID, name: item.Name})
			m.cols[colContents] = nil
			m.loading[colContents] = true
			return m, loadItemsCmd(m.token, m.selectedHubID, item.ID)
		}
		// Non-container: details already visible, no-op for right arrow.
	}
	return m, nil
}

// availableTabs returns the tabs that make sense for the currently
// selected item. The order is the same as the detailsTab enum so the
// strip renders left-to-right consistently across item types:
//
//   - DesignItem with a tipRootComponentVersion → all four tabs
//     (Details, Uses, Where Used, Drawings).
//   - DrawingItem → Details + Uses (the source design). Where Used
//     and Drawings don't apply to drawings.
//   - Other item types (BasicItem, ConfiguredDesignItem, no details)
//     → no tabs; the panel falls back to the simple Details view.
func (m Model) availableTabs() []detailsTab {
	if m.details == nil {
		return nil
	}
	switch m.details.Typename {
	case "DesignItem":
		if m.details.RootComponentVersionID != "" {
			return []detailsTab{tabDetails, tabUses, tabWhereUsed, tabDrawings}
		}
	case "DrawingItem":
		return []detailsTab{tabDetails, tabUses}
	}
	return nil
}

// tabsAvailable reports whether the tab strip should be shown for the
// currently selected item.
func (m Model) tabsAvailable() bool {
	return len(m.availableTabs()) > 0
}

// tabIsAvailable reports whether the given tab applies to the current
// item. Used by selectTab to silently ignore presses on tabs that are
// hidden for this item type.
func (m Model) tabIsAvailable(t detailsTab) bool {
	for _, a := range m.availableTabs() {
		if a == t {
			return true
		}
	}
	return false
}

// selectTab switches the active details-pane tab. The keypress is a
// no-op when the tab strip is hidden (no tabs apply to this item) or
// when the requested tab isn't available for the current item type
// (e.g. pressing "3" for Where Used while a DrawingItem is selected).
// Returns the load command for the newly active tab if its data isn't
// already cached for the current selection.
func (m Model) selectTab(t detailsTab) (Model, tea.Cmd) {
	if !m.tabIsAvailable(t) {
		return m, nil
	}
	m.detailsTab = t
	cmd := m.maybeLoadActiveTab()
	return m, cmd
}

// cycleTab returns the next or previous tab in the available list,
// wrapping around. delta is +1 for forward and -1 for back. Falls back
// to tabDetails when no tabs apply.
func (m Model) cycleTab(delta int) detailsTab {
	avail := m.availableTabs()
	if len(avail) == 0 {
		return tabDetails
	}
	for i, a := range avail {
		if a == m.detailsTab {
			n := (i + delta) % len(avail)
			if n < 0 {
				n += len(avail)
			}
			return avail[n]
		}
	}
	return avail[0]
}

// resetTabState clears per-tab loading/error/cursor/scroll values that
// belong to a specific item selection. Called whenever the underlying
// item changes (cursor moved to a new design, refresh, hub change).
// Caches survive — only the per-item view state is wiped.
func (m *Model) resetTabState() {
	m.tabLoading = [numDetailsTabs]bool{}
	m.tabErr = [numDetailsTabs]string{}
	m.tabCursors = [numDetailsTabs]int{}
	m.tabScrolls = [numDetailsTabs]int{}
}

// usesCacheKey returns the cache lookup key for the Uses tab. Two
// item types use this tab and they key their results differently:
//
//   - DesignItem: keyed by tipRootComponentVersion.id, because the
//     underlying GraphQL query is rooted at that componentVersion.
//   - DrawingItem: keyed by the DrawingItem lineage URN, because the
//     drawing's source is stable across the item's versions.
//
// usesCache stores both flavours under one map; the keys don't collide
// because the URN namespaces differ.
func (m Model) usesCacheKey() string {
	if m.details == nil {
		return ""
	}
	if m.details.Typename == "DrawingItem" {
		return m.details.ID
	}
	return m.details.RootComponentVersionID
}

// currentTabRowCount returns the number of rows in the active
// non-Details tab's content list. Returns 0 when the tab data isn't
// loaded yet, the tab has no rows, or the active tab is Details.
func (m Model) currentTabRowCount() int {
	if m.details == nil {
		return 0
	}
	cvid := m.details.RootComponentVersionID
	switch m.detailsTab {
	case tabUses:
		return len(m.usesCache[m.usesCacheKey()])
	case tabWhereUsed:
		return len(m.whereUsedCache[cvid])
	case tabDrawings:
		// Drawings cache is keyed by DesignItem.id, not cvid — see
		// loadDrawingsCmd / GetDrawingsForDesign for why.
		return len(m.drawingsCache[m.details.ID])
	}
	return 0
}

// showInLocation kicks off Show-in-Location for the row currently
// highlighted by the tab cursor (the keyboard activator; mouse
// double-click goes through the same path). For Uses / Where Used the
// target is the parent DesignItem of the row's component; for Drawings
// it is the DrawingItem itself.
func (m Model) showInLocation() (Model, tea.Cmd) {
	if !m.tabsAvailable() || m.detailsTab == tabDetails || m.details == nil {
		return m, nil
	}
	cvid := m.details.RootComponentVersionID
	cur := m.tabCursors[m.detailsTab]
	var targetID, targetName string
	switch m.detailsTab {
	case tabUses:
		items := m.usesCache[m.usesCacheKey()]
		if cur < 0 || cur >= len(items) {
			return m, nil
		}
		targetID = items[cur].DesignItemID
		targetName = items[cur].DesignItemName
		if targetName == "" {
			targetName = items[cur].Name
		}
	case tabWhereUsed:
		items := m.whereUsedCache[cvid]
		if cur < 0 || cur >= len(items) {
			return m, nil
		}
		targetID = items[cur].DesignItemID
		targetName = items[cur].DesignItemName
		if targetName == "" {
			targetName = items[cur].Name
		}
	case tabDrawings:
		items := m.drawingsCache[m.details.ID]
		if cur < 0 || cur >= len(items) {
			return m, nil
		}
		targetID = items[cur].DrawingItemID
		targetName = items[cur].Name
	}
	if targetID == "" {
		m.statusMsg = "No location available for this row"
		return m, nil
	}
	m.statusMsg = "Locating " + targetName + "…"
	return m, locateItemCmd(m.token, m.selectedHubID, targetID)
}

// handleItemLocation processes the locate-query result. If the project
// is in the current hub and visible to the user, it switches the
// Project cursor and queues the folder drill via pendingNav. Otherwise
// it surfaces a friendly status and leaves the user in place.
func (m Model) handleItemLocation(msg itemLocationLoadedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.statusMsg = "Show in Location failed: " + msg.err.Error()
		return m, nil
	}
	loc := msg.loc
	if loc == nil {
		m.statusMsg = "Show in Location: no result"
		return m, nil
	}
	if loc.HubID != "" && m.selectedHubID != "" && loc.HubID != m.selectedHubID {
		m.statusMsg = "Item is in another hub (project: " + loc.ProjectName + ")"
		return m, nil
	}
	projIdx := -1
	for i, p := range m.cols[colProjects] {
		if p.ID == loc.ProjectID {
			projIdx = i
			break
		}
	}
	if projIdx < 0 {
		m.statusMsg = "Item is in a project not visible here: " + loc.ProjectName
		return m, nil
	}
	m.cursors[colProjects] = projIdx
	m.adjustScroll(colProjects)
	m.activeCol = colContents
	m.cols[colContents] = nil
	m.loading[colContents] = true
	m.folderStack = nil
	m.selectedProjectAltID = loc.ProjectAltID
	m.pendingNav = &pendingNavState{
		targetItemID: msg.target,
		folders:      append([]api.FolderRef(nil), loc.FolderPath...),
	}
	// Switching to Details tab so the cursor change in Contents drives
	// the existing details auto-load instead of staying on a tab whose
	// list is about to become stale anyway.
	m.detailsTab = tabDetails
	return m, loadProjectContentsCmd(m.token, loc.ProjectID)
}

// maybeLoadActiveTab returns a fetch command for the active tab if its
// data isn't yet cached for the current selection. Returns nil if the
// tab is Details (already loaded eagerly with the item) or the cache
// hits. Updates loading/error flags on the model.
func (m *Model) maybeLoadActiveTab() tea.Cmd {
	if m.details == nil {
		return nil
	}
	switch m.detailsTab {
	case tabUses:
		// Uses applies to both DesignItem (sub-components) and
		// DrawingItem (source design). Cache + load command differ
		// per item type but share the same usesCache map.
		key := m.usesCacheKey()
		if key == "" {
			return nil
		}
		if _, ok := m.usesCache[key]; ok {
			m.tabLoading[tabUses] = false
			m.tabErr[tabUses] = ""
			return nil
		}
		m.tabLoading[tabUses] = true
		m.tabErr[tabUses] = ""
		if m.details.Typename == "DrawingItem" {
			return loadDrawingUsesCmd(m.token, m.selectedHubID, key)
		}
		return loadUsesCmd(m.token, key)
	case tabWhereUsed:
		// Where Used only applies to DesignItems; the strip hides this
		// tab for other item types so we shouldn't even reach here in
		// those cases. The empty-cvid guard is defensive.
		cvid := m.details.RootComponentVersionID
		if cvid == "" {
			return nil
		}
		if _, ok := m.whereUsedCache[cvid]; ok {
			m.tabLoading[tabWhereUsed] = false
			m.tabErr[tabWhereUsed] = ""
			return nil
		}
		m.tabLoading[tabWhereUsed] = true
		m.tabErr[tabWhereUsed] = ""
		return loadWhereUsedCmd(m.token, cvid)
	case tabDrawings:
		// Drawings cache + load are keyed by DesignItem.id (lineage),
		// not cvid — see GetDrawingsForDesign for the rationale.
		itemID := m.details.ID
		if _, ok := m.drawingsCache[itemID]; ok {
			m.tabLoading[tabDrawings] = false
			m.tabErr[tabDrawings] = ""
			return nil
		}
		m.tabLoading[tabDrawings] = true
		m.tabErr[tabDrawings] = ""
		return loadDrawingsCmd(m.token, m.selectedHubID, itemID)
	}
	return nil
}

// maybeLoadDetails loads details for the current item if it's a document.
// If the item's details are already cached from a prior fetch this session,
// they're served synchronously and no API call is made. The active
// details-pane tab (preserved across item changes) is re-evaluated for
// the new selection — if it's a non-Details tab and that data isn't
// cached for the new component version, an additional fetch fires.
func (m *Model) maybeLoadDetails() tea.Cmd {
	item := m.selectedItem(m.activeCol)
	if item == nil || item.IsContainer {
		m.details = nil
		m.detailsLoading = false
		m.resetTabState()
		return nil
	}
	if cached, ok := m.detailsCache[item.ID]; ok && cached != nil {
		m.details = cached
		m.detailsLoading = false
		m.resetTabState()
		return m.maybeLoadActiveTab()
	}
	m.detailsLoading = true
	m.resetTabState()
	return loadDetailsCmd(m.token, m.selectedHubID, item.ID)
}

// openInBrowser opens the selected document's permalink in the default
// browser. Only works once the details panel has loaded — that's the only
// URL source we trust, because GraphQL's item-level fusionWebUrl is the
// one the Autodesk web app actually honors. The project-level fallback URL
// and the hand-constructed fallbacks used in earlier versions point at
// routes that return "BROWSER_LOGIN_REQUIRED / WEB SESSION INVALID" for
// team hubs, so they're intentionally gone.
//
// If the user presses `o` on a container (project/folder) or before
// details have finished loading, the status bar tells them to wait for
// the details panel and no browser call is made. The key hint is shown
// at the bottom of the details panel so it only appears when `o` is in
// fact actionable.
func (m Model) openInBrowser() (Model, tea.Cmd) {
	item := m.selectedItem(m.activeCol)
	if item == nil || item.IsContainer {
		return m, nil
	}
	if m.details == nil || m.details.FusionWebURL == "" {
		if m.detailsLoading {
			m.statusMsg = "Wait for details to load before pressing o"
		} else {
			m.statusMsg = "No web URL available for this item"
		}
		return m, nil
	}
	m.statusMsg = "Opening…"
	return m, openURLCmd(m.details.FusionWebURL)
}

// openInDesktop asks the running Fusion desktop client to open the selected
// document via its local MCP server. Requires Fusion to be running.
// Blocks the call if Fusion's active hub differs from the CLI's selected hub.
func (m Model) openInDesktop() (Model, tea.Cmd) {
	item := m.selectedItem(m.activeCol)
	if item == nil || item.IsContainer {
		return m, nil
	}
	projName := ""
	if proj := m.selectedItem(colProjects); proj != nil {
		projName = proj.Name
	}
	m.statusMsg = "Opening in Fusion…"
	return m, openInFusionCmd(item.ID, m.selectedProjectAltID, projName, m.selectedHubName())
}

// insertInDesktop asks the running Fusion desktop client to insert the
// selected document as an occurrence in the currently active design, via its
// local MCP server. Requires Fusion to be running with an active design.
// Blocks the call if Fusion's active hub differs from the CLI's selected hub.
func (m Model) insertInDesktop() (Model, tea.Cmd) {
	item := m.selectedItem(m.activeCol)
	if item == nil || item.IsContainer {
		return m, nil
	}
	projName := ""
	if proj := m.selectedItem(colProjects); proj != nil {
		projName = proj.Name
	}
	m.statusMsg = "Inserting in Fusion…"
	return m, insertInFusionCmd(item.ID, m.selectedProjectAltID, projName, m.selectedHubName())
}

// openPinnedInDesktop and insertPinnedInDesktop dispatch the same Fusion
// MCP calls as openInDesktop / insertInDesktop, but sourced from the pin
// under the pins-overlay cursor instead of the active browser selection.
// Only document pins (design / drawing / configured) are valid; folder and
// project pins are no-ops with a status hint. The overlay stays open so
// the user can chain actions across multiple pins; the action result
// shows in the status line under the list.
func (m Model) openPinnedInDesktop() (Model, tea.Cmd) {
	if len(m.pins) == 0 {
		return m, nil
	}
	p := m.pins[m.pinsCursor]
	if !isDocumentPinKind(p.Kind) {
		m.statusMsg = "Open in Fusion only works on documents"
		return m, nil
	}
	if p.HubID != "" && m.selectedHubID != "" && p.HubID != m.selectedHubID {
		m.statusMsg = "Pin is in another hub: " + p.Name
		return m, nil
	}
	m.statusMsg = "Opening " + p.Name + " in Fusion…"
	return m, openInFusionCmd(p.ID, p.ProjectAltID, m.projectNameByID(p.ProjectID), m.selectedHubName())
}

func (m Model) insertPinnedInDesktop() (Model, tea.Cmd) {
	if len(m.pins) == 0 {
		return m, nil
	}
	p := m.pins[m.pinsCursor]
	if !isDocumentPinKind(p.Kind) {
		m.statusMsg = "Insert only works on documents"
		return m, nil
	}
	if p.HubID != "" && m.selectedHubID != "" && p.HubID != m.selectedHubID {
		m.statusMsg = "Pin is in another hub: " + p.Name
		return m, nil
	}
	m.statusMsg = "Inserting " + p.Name + " in Fusion…"
	return m, insertInFusionCmd(p.ID, p.ProjectAltID, m.projectNameByID(p.ProjectID), m.selectedHubName())
}

func isDocumentPinKind(kind string) bool {
	switch kind {
	case "design", "drawing", "configured":
		return true
	}
	return false
}

// projectNameByID looks up the display name of the project with the
// given lineage URN in the currently-loaded Projects column. Returns ""
// when the project isn't in the active hub — the Fusion MCP call still
// succeeds; only the hub-mismatch error message loses its friendly name.
func (m Model) projectNameByID(projectID string) string {
	if projectID == "" {
		return ""
	}
	for _, p := range m.cols[colProjects] {
		if p.ID == projectID {
			return p.Name
		}
	}
	return ""
}

// downloadStep starts the STEP-translation + download workflow for the
// currently selected design. Like [o], it requires the details panel to
// be loaded so we have the tipRootComponentVersion id needed to ask the
// MFG Data Model API for a STEP derivative. STEP export is only valid
// for DesignItems (drawings, configured designs and basic items aren't
// supported by the componentVersion-derivatives endpoint here).
//
// The translation is asynchronous: this function dispatches the initial
// GraphQL request and the Update loop drives polling + the eventual
// HTTP download to disk via stepStatusMsg / stepDoneMsg.
func (m Model) downloadStep() (Model, tea.Cmd) {
	item := m.selectedItem(m.activeCol)
	if item == nil || item.IsContainer {
		return m, nil
	}
	if m.downloadInProgress {
		m.statusMsg = "STEP download already in progress…"
		return m, nil
	}
	if m.details == nil {
		if m.detailsLoading {
			m.statusMsg = "Wait for details to load before pressing d"
		} else {
			m.statusMsg = "No details available for this item"
		}
		return m, nil
	}
	if m.details.Typename != "DesignItem" {
		m.statusMsg = "STEP download is only supported for designs"
		return m, nil
	}
	if m.details.RootComponentVersionID == "" {
		m.statusMsg = "No component version available for this design"
		return m, nil
	}
	m.downloadInProgress = true
	m.statusMsg = "Requesting STEP translation for " + m.details.Name + "…"
	return m, requestStepCmd(m.token, m.details.RootComponentVersionID, m.details.Name)
}

// selectedHubName returns the display name of the currently-selected hub,
// or an empty string if nothing is selected. Used to build helpful error
// messages when Fusion is on a different hub than the CLI.
func (m Model) selectedHubName() string {
	return m.selectedHubNameCache
}

// openInViewer opens the web viewer for the currently selected design item.
// refresh reloads the data for the active column. Refresh also drops the
// item-details cache so subsequent navigations re-fetch (a user pressing
// [r] expects fresh data, not whatever was last cached).
func (m Model) refresh() (Model, tea.Cmd) {
	m.detailsCache = make(map[string]*api.ItemDetails)
	m.usesCache = make(map[string][]api.ComponentRef)
	m.whereUsedCache = make(map[string][]api.ComponentRef)
	m.drawingsCache = make(map[string][]api.DrawingRef)
	m.detailsTab = tabDetails
	m.resetTabState()
	m.pendingNav = nil
	switch m.activeCol {
	case colProjects:
		if m.selectedHubID == "" {
			return m, nil
		}
		m.cols[colProjects] = nil
		m.loading[colProjects] = true
		return m, loadProjectsCmd(m.token, m.selectedHubID)

	case colContents:
		if len(m.folderStack) > 0 {
			// Reload current folder
			entry := m.folderStack[len(m.folderStack)-1]
			m.cols[colContents] = nil
			m.loading[colContents] = true
			return m, loadItemsCmd(m.token, m.selectedHubID, entry.id)
		}
		proj := m.selectedItem(colProjects)
		if proj == nil {
			return m, nil
		}
		m.cols[colContents] = nil
		m.loading[colContents] = true
		return m, loadProjectContentsCmd(m.token, proj.ID)
	}
	return m, nil
}

// selectHub confirms the hub selection from the overlay and loads projects.
func (m Model) selectHub() (Model, tea.Cmd) {
	if len(m.hubs) == 0 {
		return m, nil
	}
	hub := m.hubs[m.hubCursor]
	m.selectedHubID = hub.ID
	m.selectedHubAltID = hub.AltID
	m.selectedHubNameCache = hub.Name
	m.state = stateBrowsing
	m.activeCol = colProjects
	m.cols[colProjects] = nil
	m.cols[colContents] = nil
	m.contentsGen++
	m.loading[colProjects] = true
	m.details = nil
	m.folderStack = nil
	m.loadPinsForCurrentHub()
	// Reset details-pane tabs on hub change. The component-version IDs in
	// the per-tab caches belong to the previous hub's projects and aren't
	// reachable from the new hub anyway.
	m.detailsTab = tabDetails
	m.resetTabState()
	m.pendingNav = nil
	m.usesCache = make(map[string][]api.ComponentRef)
	m.whereUsedCache = make(map[string][]api.ComponentRef)
	m.drawingsCache = make(map[string][]api.DrawingRef)
	return m, loadProjectsCmd(m.token, hub.ID)
}

// adjustHubScroll keeps the hub cursor visible in the overlay.
func (m *Model) adjustHubScroll() {
	visible := m.visibleRows()
	if m.hubCursor < m.hubScroll {
		m.hubScroll = m.hubCursor
	} else if m.hubCursor >= m.hubScroll+visible {
		m.hubScroll = m.hubCursor - visible + 1
	}
}

func (m *Model) adjustPinsScroll() {
	visible := m.visibleRows()
	if m.pinsCursor < m.pinsScroll {
		m.pinsScroll = m.pinsCursor
	} else if m.pinsCursor >= m.pinsScroll+visible {
		m.pinsScroll = m.pinsCursor - visible + 1
	}
}

func (m Model) togglePin() (Model, tea.Cmd) {
	col := m.cols[m.activeCol]
	if len(col) == 0 {
		return m, nil
	}
	item := col[m.cursors[m.activeCol]]
	if !pins.IsPinnable(item.Kind) {
		m.statusMsg = "Cannot pin hubs"
		return m, nil
	}
	if pins.IsPinned(m.pins, item.ID) {
		m.pins = pins.Remove(m.pins, item.ID)
		m.statusMsg = "Unpinned: " + item.Name
	} else {
		pin := pins.Pin{
			ID:    item.ID,
			Name:  item.Name,
			Kind:  item.Kind,
			HubID: m.selectedHubID,
		}
		// Capture project context for project/folder/document navigation
		// so we can navigate without an API call later.
		if proj := m.selectedItem(colProjects); proj != nil {
			pin.ProjectID = proj.ID
			pin.ProjectAltID = proj.AltID
		}
		// Capture ancestor folder chain. For folders, include the folder
		// itself so pendingNav can drill all the way into it.
		for _, entry := range m.folderStack {
			pin.FolderPath = append(pin.FolderPath, pins.FolderRef{ID: entry.id, Name: entry.name})
		}
		if item.Kind == "folder" {
			pin.FolderPath = append(pin.FolderPath, pins.FolderRef{ID: item.ID, Name: item.Name})
		}
		m.pins = pins.Add(m.pins, pin)
		m.statusMsg = "Pinned: " + item.Name
	}
	if err := pins.Save(m.selectedHubID, m.pins); err != nil {
		m.statusMsg = "Pin save failed: " + err.Error()
	}
	return m, nil
}

func (m Model) removePinnedItem() (Model, tea.Cmd) {
	if len(m.pins) == 0 {
		return m, nil
	}
	m.pins = pins.Remove(m.pins, m.pins[m.pinsCursor].ID)
	if m.pinsCursor >= len(m.pins) && m.pinsCursor > 0 {
		m.pinsCursor--
	}
	m.adjustPinsScroll()
	_ = pins.Save(m.selectedHubID, m.pins)
	return m, nil
}

// loadPinsForCurrentHub refreshes m.pins from disk for the
// currently-selected hub and resets the overlay's cursor/scroll. Called
// whenever m.selectedHubID changes — on auto-select after hubsLoaded,
// from the hub picker, or after a hub-recovery flow — so the pins
// overlay always reflects exactly the active hub's bookmarks.
func (m *Model) loadPinsForCurrentHub() {
	if m.selectedHubID == "" {
		m.pins = nil
		m.pinsCursor = 0
		m.pinsScroll = 0
		return
	}
	loaded, _ := pins.Load(m.selectedHubID)
	m.pins = loaded
	m.pinsCursor = 0
	m.pinsScroll = 0
}

func (m Model) navigateToPinnedItem() (Model, tea.Cmd) {
	if len(m.pins) == 0 {
		return m, nil
	}
	p := m.pins[m.pinsCursor]
	m.state = stateBrowsing
	// Wrong hub: surface a friendly message and stay put.
	if p.HubID != "" && m.selectedHubID != "" && p.HubID != m.selectedHubID {
		m.statusMsg = "Pin is in another hub: " + p.Name
		return m, nil
	}
	switch p.Kind {
	case "project":
		return m.navigateToPinnedProject(p)
	case "folder":
		return m.navigateToPinnedFolder(p)
	default:
		// Documents (design, drawing, configured): use the API locate flow.
		m.statusMsg = "Locating " + p.Name + "…"
		return m, locateItemCmd(m.token, p.HubID, p.ID)
	}
}

// navigateToPinnedProject selects a pinned project in the Projects column
// and loads its root contents — no API locate call needed.
func (m Model) navigateToPinnedProject(p pins.Pin) (Model, tea.Cmd) {
	for i, proj := range m.cols[colProjects] {
		if proj.ID == p.ProjectID {
			m.cursors[colProjects] = i
			m.adjustScroll(colProjects)
			m.selectedProjectAltID = proj.AltID
			m.activeCol = colContents
			m.folderStack = nil
			m.cols[colContents] = nil
			m.loading[colContents] = true
			m.details = nil
			m.detailsScroll = 0
			m.detailsTab = tabDetails
			return m, loadProjectContentsCmd(m.token, proj.ID)
		}
	}
	m.statusMsg = "Project not in current hub: " + p.Name
	return m, nil
}

// navigateToPinnedFolder uses the folder path stored at pin time to drive
// pendingNav — no API locate call needed.
func (m Model) navigateToPinnedFolder(p pins.Pin) (Model, tea.Cmd) {
	if p.ProjectID == "" || len(p.FolderPath) == 0 {
		m.statusMsg = "No location stored for: " + p.Name
		return m, nil
	}
	projIdx := -1
	for i, proj := range m.cols[colProjects] {
		if proj.ID == p.ProjectID {
			projIdx = i
			break
		}
	}
	if projIdx < 0 {
		m.statusMsg = "Project not in current hub: " + p.Name
		return m, nil
	}
	proj := m.cols[colProjects][projIdx]
	m.cursors[colProjects] = projIdx
	m.adjustScroll(colProjects)
	m.selectedProjectAltID = proj.AltID
	m.activeCol = colContents
	m.cols[colContents] = nil
	m.loading[colContents] = true
	m.folderStack = nil
	m.details = nil
	m.detailsScroll = 0
	m.detailsTab = tabDetails
	// Convert stored folder refs to api.FolderRef for pendingNav.
	// FolderPath for a folder pin includes the folder itself as the last
	// hop, so pendingNav drills all the way into it.
	folderRefs := make([]api.FolderRef, len(p.FolderPath))
	for i, f := range p.FolderPath {
		folderRefs[i] = api.FolderRef{ID: f.ID, Name: f.Name}
	}
	m.pendingNav = &pendingNavState{folders: folderRefs}
	return m, loadProjectContentsCmd(m.token, proj.ID)
}

// selectedItem returns a pointer to the item at the cursor in a given column, or nil.
func (m *Model) selectedItem(col int) *api.NavItem {
	items := m.cols[col]
	if len(items) == 0 {
		return nil
	}
	idx := clamp(m.cursors[col], 0, len(items)-1)
	return &items[idx]
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}

	switch m.state {
	case stateSetupNeeded:
		return m.viewSetupNeeded()
	case stateLoading:
		return m.viewLoading("Starting up…")
	case stateAuthNeeded:
		return m.viewAuthNeeded()
	case stateAuthWaiting:
		return m.viewLoading("Waiting for browser authentication…")
	case stateHubSelect:
		return m.viewHubSelect()
	case stateAbout:
		return m.viewAbout()
	case stateDebug:
		return m.viewDebug()
	case statePins:
		return m.viewPins()
	case stateError:
		return m.viewError()
	}

	return m.viewBrowser()
}

func (m Model) viewLoading(msg string) string {
	content := fmt.Sprintf("\n\n  %s %s\n", m.spinner.View(), styleStatus.Render(msg))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) viewAuthNeeded() string {
	title := styleHeader.Render("fusionlocalserver")
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		styleStatus.Render("  Sign in with your Autodesk account to continue."),
		"",
		styleItemNormal.Render("  Press [Enter] to open your browser and log in."),
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Center, body)
}

func (m Model) viewHubSelect() string {
	header := styleHeader.Render("fusionlocalserver — Select Hub") +
		styleStatus.Render("  [↑↓/jk] move  [Enter] select  [r] refresh  [h] close")

	if m.hubLoading {
		body := fmt.Sprintf("\n  %s %s\n", m.spinner.View(), styleLoading.Render("Loading hubs…"))
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}

	if len(m.hubs) == 0 {
		body := styleItemDim.Render("\n  No hubs found.\n")
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}

	// Current selection indicator
	current := ""
	if m.selectedHubNameCache != "" {
		current = styleItemDim.Render("  Current: " + m.selectedHubNameCache)
	}

	visibleH := m.height - 5
	if visibleH < 1 {
		visibleH = 1
	}
	scroll := clamp(m.hubScroll, 0, max(0, len(m.hubs)-visibleH))
	end := min(scroll+visibleH, len(m.hubs))

	innerWidth := m.width - 8
	if innerWidth < 20 {
		innerWidth = 20
	}

	var sb strings.Builder
	if current != "" {
		sb.WriteString(current)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("\n")
	}
	for i := scroll; i < end; i++ {
		hub := m.hubs[i]
		icon := kindIcon(hub.Kind)
		label := truncate(icon+hub.Name, innerWidth)
		if i == m.hubCursor {
			sb.WriteString(styleItemSelected.Width(innerWidth).Render(label))
		} else {
			sb.WriteString(styleContainerItem.Width(innerWidth).Render(label))
		}
		if i < end-1 {
			sb.WriteString("\n")
		}
	}
	if scroll > 0 {
		sb.WriteString("\n" + styleItemDim.Render("  ↑ more"))
	}
	if end < len(m.hubs) {
		sb.WriteString("\n" + styleItemDim.Render("  ↓ more"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, sb.String())
}

func (m Model) viewPins() string {
	header := styleHeader.Render("fusionlocalserver — Pins") +
		styleStatus.Render("  [↑↓/ws] move  [Enter] go to item  [o] Fusion  [i] insert  [del] remove  [p] close")

	if len(m.pins) == 0 {
		body := "\n" + styleItemDim.Render("  No pins yet.") + "\n" +
			styleItemDim.Render("  Press [Shift+P] while browsing to pin a project, folder, or document.")
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}

	visibleH := m.height - 5
	if visibleH < 1 {
		visibleH = 1
	}
	scroll := clamp(m.pinsScroll, 0, max(0, len(m.pins)-visibleH))
	end := min(scroll+visibleH, len(m.pins))
	innerWidth := m.width - 8
	if innerWidth < 20 {
		innerWidth = 20
	}

	var sb strings.Builder
	sb.WriteString("\n")

	lastKind := ""
	for i := scroll; i < end; i++ {
		p := m.pins[i]
		if p.Kind != lastKind {
			if lastKind != "" {
				sb.WriteString("\n")
			}
			heading := strings.ToUpper(p.Kind[:1]) + p.Kind[1:] + "s"
			sb.WriteString(styleItemDim.Render("  — " + heading + " —"))
			sb.WriteString("\n")
			lastKind = p.Kind
		}
		icon := kindIcon(p.Kind)
		name := p.Name
		if p.Kind == "folder" {
			name += "/"
		}
		label := truncate(icon+name, innerWidth-2)
		if i == m.pinsCursor {
			sb.WriteString(styleItemSelected.Width(innerWidth).Render(label))
		} else {
			sb.WriteString(stylePinnedItem.Width(innerWidth).Render(label))
		}
		if i < end-1 {
			sb.WriteString("\n")
		}
	}
	if scroll > 0 {
		sb.WriteString("\n" + styleItemDim.Render("  ↑ more"))
	}
	if end < len(m.pins) {
		sb.WriteString("\n" + styleItemDim.Render("  ↓ more"))
	}
	if m.statusMsg != "" {
		sb.WriteString("\n\n" + styleStatus.Render("  "+m.statusMsg))
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, sb.String())
}

func (m Model) viewSetupNeeded() string {
	cfgPath := config.Path()
	title := styleHeader.Render("fusionlocalserver — developer setup")
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		styleError.Render("  No APS client_id found."),
		styleItemDim.Render("  This binary was built without an embedded client_id."),
		"",
		styleItemNormal.Render("  Option 1 — build with embedded client_id:"),
		styleItemNormal.Render(`    go build -ldflags \`),
		styleItemNormal.Render(`      "-X github.com/schneik80/fusionlocalserver/config.DefaultClientID=<id>" .`),
		"",
		styleItemNormal.Render("  Option 2 — environment variable:"),
		styleItemNormal.Render("    APS_CLIENT_ID=<id> fusionlocalserver"),
		"",
		styleItemNormal.Render("  Option 3 — config file at:"),
		styleItemNormal.Render("    "+cfgPath),
		styleItemNormal.Render(`    { "client_id": "<id>" }`),
		styleItemNormal.Render(`    { "client_id": "<id>", "region": "EMEA" }  ← non-US hubs`),
		"",
		styleItemDim.Render("  Register a public APS app at: https://aps.autodesk.com/myapps"),
		styleItemDim.Render("  Redirect URI: http://localhost:7879/callback  Scopes: data:read"),
		styleItemDim.Render("  No client_secret needed for public clients."),
		"",
		styleItemDim.Render("  Press [q] to quit."),
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Center, body)
}

func (m Model) viewDebug() string {
	header := styleHeader.Render("fusionlocalserver — debug log") +
		styleStatus.Render("  [?] close  [↑↓/jk] scroll")
	if !api.DebugEnabled() {
		body := styleItemDim.Render("\n  Debug mode is off. Re-launch with FUSIONLOCALSERVER_DEBUG=1 to enable logging.\n")
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}

	// Surface the on-disk log path so users can copy / grep / tail with
	// standard tools — the in-app overlay text is rendered, not selectable.
	var pathHint string
	if p := api.DebugLogPath(); p != "" {
		pathHint = styleItemDim.Render("  log file: " + p)
	}

	lines := api.DebugLines()
	if len(lines) == 0 {
		body := styleItemDim.Render("\n  No log entries yet.\n")
		return lipgloss.JoinVertical(lipgloss.Left, header, pathHint, body)
	}

	// Visible area: full height minus header + log-path hint + footer.
	visibleH := m.height - 4
	if visibleH < 1 {
		visibleH = 1
	}
	scroll := clamp(m.debugScroll, 0, max(0, len(lines)-visibleH))
	m.debugScroll = scroll

	end := min(scroll+visibleH, len(lines))
	var sb strings.Builder
	for _, l := range lines[scroll:end] {
		sb.WriteString(styleItemNormal.Render(l))
		sb.WriteString("\n")
	}
	footer := styleItemDim.Render(fmt.Sprintf("  lines %d–%d of %d", scroll+1, end, len(lines)))

	return lipgloss.JoinVertical(lipgloss.Left, header, pathHint, sb.String(), footer)
}

// aboutLinesCache holds the last-rendered About-screen content. It depends
// only on the version string (constant per session) and the active theme,
// so we rebuild it lazily when either changes — not every frame the
// overlay is visible.
var (
	aboutLinesCache        []string
	aboutLinesCacheVersion string
	aboutLinesCacheTheme   int
)

func renderAboutLines(version string) []string {
	if aboutLinesCache != nil &&
		aboutLinesCacheVersion == version &&
		aboutLinesCacheTheme == themeVersion {
		return aboutLinesCache
	}
	heading := styleColumnTitle.MarginBottom(0)
	lines := []string{
		styleHeader.Render("fusionlocalserver  v" + version),
		"",
		styleItemNormal.Render("  A terminal browser for Autodesk Platform Services"),
		styleItemNormal.Render("  Manufacturing Data Model — navigate Fusion hubs,"),
		styleItemNormal.Render("  projects, folders, and designs from the command line."),
		"",
		styleItemDim.Render("  https://github.com/schneik80/fusionlocalserver"),
		"",
		heading.Render("Copyright:"),
		styleItemNormal.Render("  © 2025 Kevin Schneider"),
		"",
		heading.Render("License:"),
		styleItemNormal.Render("  GNU General Public License v3.0"),
		"",
		styleItemNormal.Render("  This program is free software: you can redistribute it"),
		styleItemNormal.Render("  and/or modify it under the terms of the GNU General Public"),
		styleItemNormal.Render("  License as published by the Free Software Foundation, either"),
		styleItemNormal.Render("  version 3 of the License, or (at your option) any later"),
		styleItemNormal.Render("  version."),
		"",
		styleItemNormal.Render("  This program is distributed in the hope that it will be"),
		styleItemNormal.Render("  useful, but WITHOUT ANY WARRANTY; without even the implied"),
		styleItemNormal.Render("  warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR"),
		styleItemNormal.Render("  PURPOSE.  See the GNU General Public License for more"),
		styleItemNormal.Render("  details."),
		"",
		styleItemNormal.Render("  You should have received a copy of the GNU General Public"),
		styleItemNormal.Render("  License along with this program.  If not, see"),
		styleItemNormal.Render("  <https://www.gnu.org/licenses/>."),
		"",
		heading.Render("Open Source:"),
		styleItemNormal.Render("  This application uses the following open source libraries:"),
		"",
		styleItemNormal.Render("  Charm.sh bubbletea"),
		styleItemDim.Render("    TUI framework — github.com/charmbracelet/bubbletea"),
		styleItemDim.Render("    MIT License — © Charmbracelet, Inc."),
		"",
		styleItemNormal.Render("  Charm.sh bubbles"),
		styleItemDim.Render("    TUI components — github.com/charmbracelet/bubbles"),
		styleItemDim.Render("    MIT License — © Charmbracelet, Inc."),
		"",
		styleItemNormal.Render("  Charm.sh lipgloss"),
		styleItemDim.Render("    Terminal styling — github.com/charmbracelet/lipgloss"),
		styleItemDim.Render("    MIT License — © Charmbracelet, Inc."),
		"",
		heading.Render("Autodesk Platform Services:"),
		styleItemNormal.Render("  Powered by the APS Manufacturing Data Model API."),
		styleItemDim.Render("  Autodesk, Fusion, and related marks are trademarks of"),
		styleItemDim.Render("  Autodesk, Inc. This application is not affiliated with or"),
		styleItemDim.Render("  endorsed by Autodesk, Inc."),
		"",
		styleItemDim.Render("  https://aps.autodesk.com"),
		"",
		styleItemDim.Render("  [↑↓/jk] scroll  [a] close"),
	}
	aboutLinesCache = lines
	aboutLinesCacheVersion = version
	aboutLinesCacheTheme = themeVersion
	return lines
}

func (m Model) viewAbout() string {
	ver := m.version
	if ver == "" {
		ver = "dev"
	}
	lines := renderAboutLines(ver)

	// Scroll window
	visibleH := m.height - 2
	if visibleH < 1 {
		visibleH = 1
	}
	maxScroll := max(0, len(lines)-visibleH)
	scroll := clamp(m.aboutScroll, 0, maxScroll)

	end := min(scroll+visibleH, len(lines))
	var sb strings.Builder
	for _, l := range lines[scroll:end] {
		sb.WriteString(l)
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m Model) viewError() string {
	msg := "unknown error"
	if m.err != nil {
		msg = m.err.Error()
	}
	hint := "[r] Retry   [q] Quit"
	if isAuthError(m.err) {
		hint = "[r] Sign in again   [q] Quit"
	}
	content := styleError.Render("Error: " + msg + "\n\n" + hint)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// isAuthError reports whether an error is almost certainly an expired or
// invalid access token, so the error screen can steer the user toward
// re-authenticating instead of a simple retry.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "token may be expired") ||
		strings.Contains(msg, "401")
}

// recoverFromError resets the model from stateError back to the beginning of
// the init flow. For auth errors it also deletes the on-disk token file so
// the next checkTokensCmd call is guaranteed to prompt for a fresh login
// instead of reusing a server-rejected token. For any other error it simply
// re-runs the same init sequence the process would run on startup.
func (m Model) recoverFromError() (Model, tea.Cmd) {
	if isAuthError(m.err) {
		_ = auth.DeleteTokens()
	}
	// Reset transient state so the UI comes back to a clean starting point.
	m.err = nil
	m.token = ""
	m.hubs = nil
	m.hubCursor = 0
	m.hubScroll = 0
	m.hubLoading = false
	m.selectedHubID = ""
	m.selectedHubAltID = ""
	m.selectedHubNameCache = ""
	m.selectedProjectAltID = ""
	m.folderStack = nil
	m.contentsGen++
	m.loadPinsForCurrentHub()
	m.cols = [numCols][]api.NavItem{}
	m.cursors = [numCols]int{}
	m.scrolls = [numCols]int{}
	m.loading = [numCols]bool{}
	m.activeCol = colProjects
	m.details = nil
	m.detailsLoading = false
	m.detailsScroll = 0
	m.detailsCache = make(map[string]*api.ItemDetails)
	m.statusMsg = ""
	m.state = stateLoading
	return m, tea.Batch(m.spinner.Tick, checkTokensCmd(m.clientID, m.clientSecret))
}

// fitFooterLine composes the footer text (help on the left, version right-
// aligned) so that it fits on a single line of the given display width.
// If the help text is too wide, it is truncated with a trailing ellipsis so
// the version remains visible. If even the version can't fit, it is dropped.
// Both inputs are measured with lipgloss.Width so multi-byte glyphs like
// ↑↓←→ contribute 1 display column rather than their 3-byte UTF-8 length.
func fitFooterLine(help, version string, width int) string {
	const minGap = 2
	verW := lipgloss.Width(version)
	if width <= verW+minGap {
		// No room for any help text — show just the version (or truncated).
		return truncateDisplay(version, width)
	}
	maxHelpW := width - verW - minGap
	helpW := lipgloss.Width(help)
	if helpW > maxHelpW {
		help = truncateDisplay(help, maxHelpW)
		helpW = lipgloss.Width(help)
	}
	gap := width - helpW - verW
	if gap < minGap {
		gap = minGap
	}
	return help + strings.Repeat(" ", gap) + version
}

// truncateDisplay trims a string to fit in at most maxWidth display columns,
// appending an ellipsis when the input was actually truncated. Uses
// lipgloss.Width so multi-byte glyphs are counted correctly.
//
// Implementation: a true binary search over the rune-length prefix that
// fits with room for the trailing ellipsis. The previous one-rune-at-a-
// time loop ran O(n) lipgloss.Width measurements; this runs O(log n).
func truncateDisplay(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return "…"
	}
	runes := []rune(s)
	// Find the largest k such that lipgloss.Width(string(runes[:k]) + "…") ≤ maxWidth.
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if lipgloss.Width(string(runes[:mid])+"…") <= maxWidth {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return "…"
	}
	return string(runes[:lo]) + "…"
}

func (m Model) viewBrowser() string {
	// Reserve rows: 1 header + 2 footer (border+text) + 2 column border = 5
	const fixedRows = 5
	colHeight := m.height - fixedRows
	if colHeight < 3 {
		colHeight = 3
	}

	// 3-panel layout: Projects | Contents | Details
	// Details gets ~35% of the width; the 2 nav columns split the rest.
	detailsWidth := (m.width * 35) / 100
	navWidth := m.width - detailsWidth - 2
	colWidth := (navWidth - 4) / numCols
	if colWidth < 10 {
		colWidth = 10
	}
	navInner := colWidth - 4
	if navInner < 4 {
		navInner = 4
	}
	detailsInner := detailsWidth - 4
	if detailsInner < 4 {
		detailsInner = 4
	}
	sc := m.ensureStyleCache(navInner, detailsInner)
	cols := make([]string, numCols)
	titles := []string{"Projects", "Contents"}
	for i := 0; i < numCols; i++ {
		cols[i] = m.renderColumn(i, titles[i], colWidth, colHeight, sc)
	}
	detailsCol := m.viewDetailsColumn(detailsWidth, colHeight, sc)
	browserRow := lipgloss.JoinHorizontal(lipgloss.Top,
		append(cols, detailsCol)...)

	// Breadcrumb header: Hub › Project › Folder(s) › Document
	// The crumbs are built with buildBreadcrumb so the same logic drives
	// both the rendered string and the mouse hit-test regions.
	breadcrumb, _ := m.buildBreadcrumb(breadcrumbXOffset())
	headerParts := "fusionlocalserver"
	if breadcrumb != "" {
		headerParts += "  " + breadcrumb
	}
	if m.statusMsg != "" {
		headerParts += "  " + m.statusMsg
	}
	header := lipgloss.NewStyle().MaxWidth(m.width).Render(
		styleHeader.Render(headerParts),
	)

	// Footer: help text on the left, version right-aligned. The help text
	// MUST fit on a single line — if the footer wraps to a second row, the
	// total vertical layout exceeds m.height and the header scrolls off
	// the top of the terminal. We measure using lipgloss.Width (display
	// width, not bytes — the glyphs like ↑↓←→ are multi-byte UTF-8) and
	// truncate the help text with an ellipsis if needed.
	mouseLabel := "[m] mouse:on"
	if !m.mouseEnabled {
		mouseLabel = "[m] mouse:off"
	}
	helpText := "[↑↓/ws] move  [←→/ad] nav  [h] hubs  [shift+p] pin  [p] pins  [r] refresh  [t] theme  " + mouseLabel + "  [shift+a] about  [q] quit"
	// contentWidth is the writable area inside styleFooter's border+padding:
	// border(none left/right) + padding(0,1) = 2 columns reserved. The border
	// is drawn only on the top, so only horizontal padding consumes columns.
	contentWidth := m.width - 2 - 2
	if contentWidth < 1 {
		contentWidth = 1
	}
	footerLine := fitFooterLine(helpText, m.version, contentWidth)
	footer := styleFooter.Width(m.width - 2).Render(footerLine)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		browserRow,
		footer,
	)
}

func (m Model) renderColumn(col int, title string, width, height int, sc *styleCache) string {
	innerWidth := sc.navInner

	var sb strings.Builder

	// Title row
	sb.WriteString(sc.columnTitleNav.Render(title))
	sb.WriteString("\n")

	// Loading indicator
	if m.loading[col] {
		sb.WriteString(m.spinner.View())
		sb.WriteString(styleLoading.Render(" Loading…"))
	} else {
		items := m.cols[col]
		if len(items) == 0 {
			// Distinguish "never loaded" (nil) from "loaded but no content" (non-nil empty slice).
			if col == colContents && items != nil {
				sb.WriteString(sc.emptyNav.Render("No designs found."))
				sb.WriteString("\n")
				sb.WriteString(sc.emptyNav.Render("Project may contain legacy"))
				sb.WriteString("\n")
				sb.WriteString(sc.emptyNav.Render("or non-Fusion content."))
			} else {
				sb.WriteString(sc.emptyNav.Render("(empty)"))
			}
		} else {
			visibleRows := height - 3 // title + bottom margin
			if visibleRows < 1 {
				visibleRows = 1
			}
			scroll := m.scrolls[col]
			cursor := m.cursors[col]

			end := scroll + visibleRows
			if end > len(items) {
				end = len(items)
			}

			for i := scroll; i < end; i++ {
				item := items[i]
				name, suffix := itemLabel(item, innerWidth-2)
				// styledSuffix is rendered inline with a dim foreground
				// so the asm/part tag reads as secondary metadata next
				// to the row's primary name. lipgloss preserves the
				// inner ANSI codes when the outer row style is applied
				// over the concatenated string, so each segment keeps
				// its own color.
				styledSuffix := ""
				if suffix != "" {
					styledSuffix = styleSubtypeDim.Render(suffix)
				}
				label := name + styledSuffix

				active := col == m.activeCol
				selected := i == cursor

				var line string
				switch {
				case active && selected:
					line = sc.itemSelectedNav.Render(label)
				case selected:
					line = sc.itemSelectedAccent.Render(label)
				default:
					if pins.IsPinned(m.pins, item.ID) {
						icon := kindIcon(item.Kind)
						pname := item.Name
						if item.Kind == "folder" {
							pname += "/"
						}
						pinnedLabel := "★ " + truncate(icon+pname, innerWidth-4-displayWidth(suffix)) + styledSuffix
						line = sc.pinnedItemNav.Render(pinnedLabel)
					} else if item.IsContainer {
						line = sc.containerItemNav.Render(label)
					} else {
						line = sc.documentItemNav.Render(label)
					}
				}
				sb.WriteString(line)
				if i < end-1 {
					sb.WriteString("\n")
				}
			}

			// Scroll indicators
			if scroll > 0 {
				sb.WriteString("\n" + styleItemDim.Render("  ↑ more"))
			}
			if end < len(items) {
				sb.WriteString("\n" + styleItemDim.Render("  ↓ more"))
			}
		}
	}

	content := sb.String()
	style := styleColumnInactive
	if col == m.activeCol {
		style = styleColumnActive
	}
	return style.Width(width).Height(height).Render(content)
}

// ---------------------------------------------------------------------------
// Details column
// ---------------------------------------------------------------------------

func (m Model) viewDetailsColumn(width, height int, sc *styleCache) string {
	inner := sc.detailsInner

	var sb strings.Builder
	// Header row: tab strip when tabs are available for this item, else
	// the simple "Details" title (drawings, configured designs, no item).
	if m.tabsAvailable() {
		sb.WriteString(renderTabStrip(m.detailsTab, m.availableTabs(), inner))
		sb.WriteString("\n\n") // tabs use no MarginBottom; add blank row to match title spacing
	} else {
		sb.WriteString(sc.columnTitleDetails.Render("Details"))
		sb.WriteString("\n")
	}

	// A document is "actionable" in Fusion when the details panel is
	// populated for a non-container item. When true, we pin hint text for
	// the [f] open / [i] insert commands at the bottom of the panel so the
	// user knows these commands target this document.
	showFusionHints := !m.detailsLoading && m.details != nil
	hintReserved := 0
	if showFusionHints {
		hintReserved = 2 // blank separator + hint line
	}

	// Total lines available inside the column body (after title + borders).
	// Tab strip uses 2 lines (strip + blank) like the original title (title + MarginBottom).
	bodyH := height - 3
	if bodyH < 1 {
		bodyH = 1
	}
	// Space for scrollable details content, excluding reserved hint rows.
	visibleH := bodyH - hintReserved
	if visibleH < 1 {
		visibleH = 1
	}

	usedLines := 0
	switch {
	case m.detailsLoading:
		sb.WriteString(m.spinner.View())
		sb.WriteString(styleLoading.Render(" Loading…"))
		usedLines = 1
	case m.details == nil:
		sb.WriteString(sc.itemDimDetails.Render("No item selected"))
		usedLines = 1
	case m.detailsTab == tabDetails || !m.tabsAvailable():
		d := m.details
		lines := m.detailLines(d, inner, sc)
		scroll := clamp(m.detailsScroll, 0, max(0, len(lines)-visibleH))
		end := min(scroll+visibleH, len(lines))

		for i, l := range lines[scroll:end] {
			sb.WriteString(l)
			if i < end-scroll-1 {
				sb.WriteString("\n")
			}
			usedLines++
		}
		if scroll > 0 {
			sb.WriteString("\n" + styleItemDim.Render("  ↑ more"))
			usedLines++
		}
		if end < len(lines) {
			sb.WriteString("\n" + styleItemDim.Render("  ↓ more"))
			usedLines++
		}
	default:
		// Uses / Where Used / Drawings tab content. tabContentLines
		// already windows internally, so we just emit what it returns.
		// "↑/↓ N more" indicators show whether scroll is possible above
		// or below the current view.
		rows := m.currentTabRowCount()
		cursor := m.tabCursors[m.detailsTab]
		scroll := m.tabScrolls[m.detailsTab]
		hasUp := scroll > 0
		hasDown := false
		if rows > 0 {
			// Best-effort check: if there are more rows past the
			// current scroll than the window can fit (approx. 1 ref
			// per 1.5 lines), there's content below.
			approxRefsVisible := visibleH / 2
			if approxRefsVisible < 1 {
				approxRefsVisible = 1
			}
			hasDown = scroll+approxRefsVisible < rows
		}
		availH := visibleH
		if hasUp {
			availH--
		}
		if hasDown {
			availH--
		}
		if availH < 1 {
			availH = 1
		}
		lines := m.tabContentLines(inner, availH, sc)
		if hasUp {
			sb.WriteString(styleItemDim.Render("  ↑ more"))
			sb.WriteString("\n")
			usedLines++
		}
		for i, l := range lines {
			sb.WriteString(l)
			if i < len(lines)-1 {
				sb.WriteString("\n")
			}
			usedLines++
		}
		if hasDown {
			sb.WriteString("\n" + styleItemDim.Render(fmt.Sprintf("  ↓ %d more", rows-1-cursor)))
			usedLines++
		}
	}

	if showFusionHints {
		// Pad with blank lines so the hint pins to the bottom of the panel.
		pad := visibleH - usedLines
		if pad < 0 {
			pad = 0
		}
		for i := 0; i < pad; i++ {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
		var hint string
		if m.tabsAvailable() && m.detailsTab != tabDetails {
			// Uses / Where Used / Drawings tab — show navigation hints for the tab rows.
			hint = "[↑↓/ws] select  [enter] show in location  [1-4] tabs"
		} else {
			hint = "[u] web  [o] Fusion  [i] insert"
			// [shift+d] download is only meaningful for designs — drawings and other
			// item kinds don't have a component version we can hand to the
			// STEP derivative endpoint.
			if m.details.Typename == "DesignItem" {
				hint += "  [shift+d] step"
			}
			if m.tabsAvailable() {
				hint += "  [1-4] tabs"
			}
		}
		sb.WriteString(sc.itemDimDetails.Render(hint))
	}

	return styleColumnInactive.Width(width).Height(height).Render(sb.String())
}

// renderTabStrip builds the header row showing only the tabs available
// for the current item (DesignItem → 4 tabs, DrawingItem → 2 tabs).
// Active tab is rendered in accent+bold, inactives in muted, separated
// by " │ ". Switches to abbreviated labels when the inner width can't
// fit the full label set.
func renderTabStrip(active detailsTab, tabs []detailsTab, innerWidth int) string {
	const fullSp = " │ "
	// Compute the full-label width on the fly because it depends on
	// which tabs are visible — DrawingItem's 2-tab strip is much
	// narrower than DesignItem's 4-tab strip.
	fullW := 0
	for i, t := range tabs {
		fullW += lipgloss.Width(t.label())
		if i < len(tabs)-1 {
			fullW += lipgloss.Width(fullSp)
		}
	}
	useFull := innerWidth >= fullW
	sep := styleTabSep.Render(fullSp)
	var parts []string
	for _, t := range tabs {
		var lbl string
		if useFull {
			lbl = t.label()
		} else {
			lbl = t.labelShort()
		}
		if t == active {
			parts = append(parts, styleTabActive.Render(lbl))
		} else {
			parts = append(parts, styleTabInactive.Render(lbl))
		}
	}
	return strings.Join(parts, sep)
}

// tabContentLines renders the active non-Details tab's content list,
// windowed to visibleH lines starting from m.tabScrolls. The active row
// (m.tabCursors[tab]) is rendered with the selected-row style so the
// keyboard cursor + mouse single-click highlight is visible.
func (m Model) tabContentLines(width, visibleH int, sc *styleCache) []string {
	if m.details == nil {
		return []string{sc.itemDimDetails.Render("No item selected")}
	}
	cvid := m.details.RootComponentVersionID
	tab := m.detailsTab

	if m.tabLoading[tab] {
		return []string{m.spinner.View() + styleLoading.Render(" Loading…")}
	}
	if e := m.tabErr[tab]; e != "" {
		return []string{styleError.Render(truncate("Error: "+e, width))}
	}

	cursor := m.tabCursors[tab]
	scroll := m.tabScrolls[tab]

	switch tab {
	case tabUses:
		items, ok := m.usesCache[m.usesCacheKey()]
		if !ok {
			return []string{sc.itemDimDetails.Render("(not loaded)")}
		}
		if len(items) == 0 {
			emptyMsg := "No sub-components"
			if m.details.Typename == "DrawingItem" {
				emptyMsg = "No source design"
			}
			return []string{sc.itemDimDetails.Render(emptyMsg)}
		}
		return componentRefLines(items, cursor, scroll, visibleH, width)
	case tabWhereUsed:
		items, ok := m.whereUsedCache[cvid]
		if !ok {
			return []string{sc.itemDimDetails.Render("(not loaded)")}
		}
		if len(items) == 0 {
			return []string{sc.itemDimDetails.Render("Not referenced by any other design")}
		}
		return componentRefLines(items, cursor, scroll, visibleH, width)
	case tabDrawings:
		items, ok := m.drawingsCache[m.details.ID]
		if !ok {
			return []string{sc.itemDimDetails.Render("(not loaded)")}
		}
		if len(items) == 0 {
			return []string{sc.itemDimDetails.Render("No drawings reference this design")}
		}
		return drawingRefLines(items, cursor, scroll, visibleH, width)
	}
	return nil
}

// componentRefLines renders a list of ComponentRef rows starting at
// `scroll` and stopping once visibleH lines are filled. The row at
// index `cursor` is rendered in selected style.
func componentRefLines(refs []api.ComponentRef, cursor, scroll, visibleH, width int) []string {
	var lines []string
	if scroll < 0 {
		scroll = 0
	}
	for i := scroll; i < len(refs) && len(lines) < visibleH; i++ {
		r := refs[i]
		head := r.Name
		if head == "" {
			head = r.DesignItemName
		}
		if r.PartNumber != "" && r.PartNumber != head {
			head = head + "  " + styleDetailKey.Render(r.PartNumber)
		}
		primary := styleItemNormal
		if i == cursor {
			primary = styleItemSelected
		}
		lines = append(lines, primary.Render(truncate(head, width)))
		if len(lines) >= visibleH {
			break
		}
		if r.DesignItemName != "" && r.DesignItemName != r.Name {
			lines = append(lines, styleItemDim.Render(truncate("  in "+r.DesignItemName, width)))
		}
	}
	return lines
}

// drawingRefLines renders a windowed list of DrawingRef rows. Same
// shape as componentRefLines: primary line = drawing name, secondary
// (dim) line = modified-on / modified-by metadata.
func drawingRefLines(refs []api.DrawingRef, cursor, scroll, visibleH, width int) []string {
	var lines []string
	if scroll < 0 {
		scroll = 0
	}
	for i := scroll; i < len(refs) && len(lines) < visibleH; i++ {
		r := refs[i]
		primary := styleItemNormal
		if i == cursor {
			primary = styleItemSelected
		}
		lines = append(lines, primary.Render(truncate(r.Name, width)))
		if len(lines) >= visibleH {
			break
		}
		var sub string
		if !r.ModifiedOn.IsZero() {
			sub = "  " + r.ModifiedOn.Format("Jan 02 2006")
		}
		if r.ModifiedBy != "" {
			if sub == "" {
				sub = "  " + r.ModifiedBy
			} else {
				sub += "  " + r.ModifiedBy
			}
		}
		if sub != "" {
			lines = append(lines, styleItemDim.Render(truncate(sub, width)))
		}
	}
	return lines
}

// detailLines returns pre-rendered lines for the details panel, memoised on
// the (details pointer, width, theme) tuple. Called from viewDetailsColumn
// every frame; the cache means the actual rendering only runs when the
// selected item, terminal width, or theme changes.
func (m Model) detailLines(d *api.ItemDetails, width int, sc *styleCache) []string {
	if sc.detailLinesPtr == d && sc.detailLinesWidth == width && sc.detailLines != nil {
		return sc.detailLines
	}
	lines := buildDetailLines(d, width, sc)
	sc.detailLines = lines
	sc.detailLinesPtr = d
	sc.detailLinesWidth = width
	return lines
}

// buildDetailLines returns pre-rendered lines for the details panel.
func buildDetailLines(d *api.ItemDetails, width int, sc *styleCache) []string {
	label := func(k, v string) string {
		if v == "" {
			return ""
		}
		key := styleDetailKey.Render(k)
		return truncate(key+" "+v, width)
	}
	heading := func(s string) string {
		return sc.columnTitleHeading.Render(s + ":")
	}
	var lines []string
	add := func(s string) {
		if s != "" {
			lines = append(lines, s)
		}
	}

	// Name
	add(truncate(d.Name, width))
	add("")

	// Core metadata
	add(label("Size    ", formatSize(d.Size)))
	if d.VersionNumber > 0 {
		add(label("Version ", fmt.Sprintf("v%d", d.VersionNumber)))
	}
	add("")

	// Created / modified
	if !d.CreatedOn.IsZero() {
		add(heading("Created"))
		add(styleItemNormal.Render("  " + d.CreatedOn.Format("Jan 02 2006")))
		if d.CreatedBy != "" {
			add(styleItemNormal.Render("  " + d.CreatedBy))
		}
		add("")
	}
	if !d.ModifiedOn.IsZero() {
		add(heading("Modified"))
		add(styleItemNormal.Render("  " + d.ModifiedOn.Format("Jan 02 2006")))
		if d.ModifiedBy != "" {
			add(styleItemNormal.Render("  " + d.ModifiedBy))
		}
		add("")
	}

	// Design-specific fields
	if d.PartNumber != "" || d.PartDesc != "" || d.Material != "" {
		add(heading("Component"))
		add(label("Part No. ", d.PartNumber))
		add(label("Desc     ", d.PartDesc))
		add(label("Material ", d.Material))
		if d.IsMilestone {
			add(styleItemNormal.Render("  ★ Milestone"))
		}
		add("")
	}

	// Version history
	if len(d.Versions) > 0 {
		add(heading("Versions"))
		for i := len(d.Versions) - 1; i >= 0; i-- {
			if len(d.Versions)-1-i >= 10 {
				add(styleItemDim.Render(fmt.Sprintf("  … %d more", len(d.Versions)-10)))
				break
			}
			v := d.Versions[i]
			date := ""
			if !v.CreatedOn.IsZero() {
				date = v.CreatedOn.Format("Jan 02 2006")
			}
			header := fmt.Sprintf("  v%-3d  %s", v.Number, date)
			if v.CreatedBy != "" {
				header = truncate(header+"  "+v.CreatedBy, width)
			}
			add(styleItemNormal.Render(header))
			if v.Comment != "" {
				add(styleItemDim.Render(truncate("        "+v.Comment, width)))
			}
		}
		add("")
	}

	return lines
}

// formatSize converts a raw size string (bytes as string) to human-readable.
func formatSize(s string) string {
	if s == "" {
		return ""
	}
	bytes, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return s // not numeric; return as-is
	}
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (m Model) visibleRows() int {
	const fixedRows = 8 // header + footer + borders + title
	v := m.height - fixedRows
	if v < 1 {
		v = 1
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// itemLabel builds the display label for a nav item with a given max
// display width, returned as (name, suffix). The caller is expected to
// concatenate them — possibly with a different style applied to suffix —
// so the row's primary text and the dimmer assembly/part tag can render
// in different colors without losing the truncation budget that
// itemLabel computed.
//
// Folders get a trailing "/" appended to name to distinguish them from
// documents.
//
// For design items, the async assembly classifier (see ClassifyAssembly +
// classifyContentsCmd) refines item.Subtype to "assembly" or "part" a
// short time after the items list lands; suffix carries the inline tag
// so the document type is visible at a glance. An unclassified design
// (Subtype == "") returns suffix = "" — the row falls back to the
// generic design icon until the refinement arrives.
//
// suffix already includes its leading spacer; do not add extra padding
// when concatenating. displayWidth(suffix) is the visual column count
// reserved out of maxWidth before truncating name.
func itemLabel(item api.NavItem, maxWidth int) (name, suffix string) {
	icon := kindIcon(item.Kind)
	suffix = subtypeSuffix(item)
	if item.Kind == "folder" {
		// Reserve 1 col for the trailing slash.
		name = truncate(icon+item.Name, maxWidth-1-displayWidth(suffix)) + "/"
		return name, suffix
	}
	name = truncate(icon+item.Name, maxWidth-displayWidth(suffix))
	return name, suffix
}

// subtypeSuffix returns the inline tag appended to a Contents-row
// label after the file name. The tag distinguishes file types at a
// glance and is rendered in a dimmer color (see styleSubtypeDim in
// ui/styles.go) so it reads as secondary metadata.
//
// Design rows pick up "asm" / "part" asynchronously after the
// classifier returns — an empty Subtype on a design means
// "not yet classified," and the renderer falls back to the generic
// design icon until the refinement message lands.
//
// Drawing rows are sub-typed at items-list time via the filename
// extension (.f2t → template, otherwise dwg) — see
// drawingSubtypeFromExtension in api/queries.go.
//
// PCB / Schematic / ECAD rows carry their type via Kind itself, so
// the tag falls out of Kind alone — Subtype is unused for those.
func subtypeSuffix(item api.NavItem) string {
	switch item.Kind {
	case "design":
		switch item.Subtype {
		case "assembly":
			return "  · asm"
		case "part":
			return "  · part"
		}
	case "drawing":
		switch item.Subtype {
		case "template":
			return "  · template"
		case "dwg":
			return "  · dwg"
		}
	case "pcb":
		return "  · pcb"
	case "schematic":
		return "  · schem"
	case "ecad":
		return "  · ecad"
	}
	return ""
}

// displayWidth approximates the visual column width of an ASCII suffix.
// We use byte count because the only suffixes produced by
// subtypeSuffix are ASCII; if we ever switch to non-ASCII chars this
// needs a real width function.
func displayWidth(s string) int { return len(s) }

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	// Byte length is an upper bound on rune count: any string with len(s)
	// bytes ≤ max can't have more than max runes, so we can skip the
	// []rune allocation on the common no-truncation path.
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// itemWebURL, accURL, and fusionURL used to hand-construct browser URLs
// as fallbacks for openInBrowser. They pointed at "https://autodesk360.com"
// and "https://acc.autodesk.com" with no hub subdomain, which Autodesk's
// team web app rejects with a BROWSER_LOGIN_REQUIRED JSON error. Only the
// per-item fusionWebUrl from GraphQL is trusted now; the fallbacks were
// removed in v2.0.5.
