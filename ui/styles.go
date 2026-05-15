package ui

import "github.com/charmbracelet/lipgloss"

// ---------------------------------------------------------------------------
// Themes
// ---------------------------------------------------------------------------

type colorTheme struct {
	name        string
	accent      lipgloss.Color
	subtle      lipgloss.Color
	muted       lipgloss.Color
	fg          lipgloss.Color
	errCol      lipgloss.Color
	detailKey   lipgloss.Color // label color for detail panel metadata keys
	loading     lipgloss.Color // color for loading spinners and empty state text
	containerFg lipgloss.Color // hubs, projects, folders
	documentFg  lipgloss.Color // designs, drawings
	pinnedFg    lipgloss.Color // pinned items — gold/starred accent
}

var themes = []colorTheme{
	{
		// Rust — original warm orange palette
		name:      "Rust",
		accent:    lipgloss.Color("#C05A1F"),
		subtle:    lipgloss.Color("#555555"),
		muted:     lipgloss.Color("#888888"),
		fg:        lipgloss.Color("#FFFFFF"),
		errCol:    lipgloss.Color("#FF5555"),
		detailKey:   lipgloss.Color("#888888"),
		loading:     lipgloss.Color("#888888"),
		containerFg: lipgloss.Color("#89B4D4"), // steel blue — complement to rust orange
		documentFg:  lipgloss.Color("#FFFFFF"), // white
		pinnedFg:    lipgloss.Color("#D4AC0D"), // warm gold
	},
	{
		// Mono — greyscale only
		name:        "Mono",
		accent:      lipgloss.Color("#CCCCCC"),
		subtle:      lipgloss.Color("#444444"),
		muted:       lipgloss.Color("#777777"),
		fg:          lipgloss.Color("#FFFFFF"),
		errCol:      lipgloss.Color("#AAAAAA"),
		detailKey:   lipgloss.Color("#999999"),
		loading:     lipgloss.Color("#777777"),
		containerFg: lipgloss.Color("#EEEEEE"), // bright grey — structural elements
		documentFg:  lipgloss.Color("#AAAAAA"), // mid grey    — data items recede
		pinnedFg:    lipgloss.Color("#E8D44D"), // bright yellow — only non-grey, immediately distinct
	},
	{
		// System — ANSI color tokens; inherits the terminal's own color scheme
		name:        "System",
		accent:      lipgloss.Color("6"),  // ANSI cyan         — high contrast accent
		subtle:      lipgloss.Color("5"),  // ANSI purple — borders / footer border
		muted:       lipgloss.Color("5"),  // ANSI purple — footer text / dim
		fg:          lipgloss.Color("7"),  // ANSI white        — normal foreground
		errCol:      lipgloss.Color("1"),  // ANSI red
		detailKey:   lipgloss.Color("6"),  // ANSI cyan         — high contrast label color
		loading:     lipgloss.Color("3"),  // ANSI yellow       — loading / empty state
		containerFg: lipgloss.Color("2"),  // ANSI green        — directories in ls
		documentFg:  lipgloss.Color("7"),  // ANSI white        — regular files
		pinnedFg:    lipgloss.Color("3"),  // ANSI yellow       — starred/important
	},
}

var activeThemeIdx = 0

// themeVersion is bumped each time applyTheme runs so caches keyed off the
// active palette can detect staleness without snapshotting every Style.
var themeVersion = 0

// cycleTheme advances to the next theme, rebuilds all styles, and returns the
// new theme name for display in the status bar.
func cycleTheme() string {
	activeThemeIdx = (activeThemeIdx + 1) % len(themes)
	applyTheme(themes[activeThemeIdx])
	return themes[activeThemeIdx].name
}

// ---------------------------------------------------------------------------
// Style variables — rebuilt on every theme change via applyTheme
// ---------------------------------------------------------------------------

var (
	colorAccent   lipgloss.Color
	colorSubtle   lipgloss.Color
	colorMuted    lipgloss.Color
	colorSelected lipgloss.Color

	styleHeader         lipgloss.Style
	styleStatus         lipgloss.Style
	styleColumnActive   lipgloss.Style
	styleColumnInactive lipgloss.Style
	styleColumnTitle    lipgloss.Style
	styleItemSelected   lipgloss.Style
	styleItemNormal     lipgloss.Style
	styleItemDim        lipgloss.Style
	styleFooter         lipgloss.Style
	styleLoading        lipgloss.Style
	styleError          lipgloss.Style
	styleKindBadge       lipgloss.Style
	styleDetailKey       lipgloss.Style
	styleEmpty           lipgloss.Style
	styleContainerItem   lipgloss.Style
	styleDocumentItem    lipgloss.Style
	stylePinnedItem      lipgloss.Style
	styleSubtypeDim      lipgloss.Style
	styleTabActive       lipgloss.Style
	styleTabInactive     lipgloss.Style
	styleTabSep          lipgloss.Style
)

func applyTheme(t colorTheme) {
	themeVersion++
	colorAccent = t.accent
	colorSubtle = t.subtle
	colorMuted = t.muted
	colorSelected = t.fg

	styleHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorAccent).
		Padding(0, 1)

	styleStatus = lipgloss.NewStyle().
		Foreground(colorMuted).
		Italic(true)

	styleColumnActive = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(0, 1)

	styleColumnInactive = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSubtle).
		Padding(0, 1)

	styleColumnTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorAccent).
		MarginBottom(1)

	styleItemSelected = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorSelected).
		Background(colorAccent).
		Padding(0, 1)

	styleItemNormal = lipgloss.NewStyle().
		Foreground(colorSelected).
		Padding(0, 1)

	styleItemDim = lipgloss.NewStyle().
		Foreground(colorMuted).
		Padding(0, 1)

	styleFooter = lipgloss.NewStyle().
		Foreground(colorMuted).
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(colorSubtle).
		Padding(0, 1)

	styleLoading = lipgloss.NewStyle().
		Foreground(t.loading).
		Italic(true).
		Padding(0, 1)

	styleEmpty = lipgloss.NewStyle().
		Foreground(t.loading).
		Padding(0, 1)

	styleError = lipgloss.NewStyle().
		Foreground(t.errCol).
		Bold(true).
		Padding(1, 2)

	styleKindBadge = lipgloss.NewStyle().
		Foreground(colorMuted).
		Faint(true)

	styleDetailKey = lipgloss.NewStyle().
		Foreground(t.detailKey)

	styleContainerItem = lipgloss.NewStyle().
		Foreground(t.containerFg).
		Padding(0, 1)

	styleDocumentItem = lipgloss.NewStyle().
		Foreground(t.documentFg).
		Padding(0, 1)

	stylePinnedItem = lipgloss.NewStyle().
		Foreground(t.pinnedFg).
		Padding(0, 1)

	// Inline foreground-only style for the assembly/part suffix
	// appended after a design's name in the Contents column. Just a
	// dim color — no padding, no decorations — so it composes cleanly
	// inside a row label string while keeping the suffix visually
	// subordinate to the file name.
	styleSubtypeDim = lipgloss.NewStyle().
		Foreground(colorMuted).
		Faint(true)

	styleTabActive = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorAccent)

	styleTabInactive = lipgloss.NewStyle().
		Foreground(colorMuted)

	styleTabSep = lipgloss.NewStyle().
		Foreground(colorSubtle)
}

func init() {
	applyTheme(themes[0])
}

// ---------------------------------------------------------------------------
// Icons
// ---------------------------------------------------------------------------

// kindIcon returns a short prefix icon for the item kind.
//
// The Fusion-glyph icons (folder, design/configured, drawing, plus
// the electronics types added in v6) are all rendered in the
// SymbolsNerdFontMono / Fusion font that the app expects in the
// terminal; consumers without that font see fallback glyphs. The new
// electronics types (schematic / pcb / ecad) reuse the design glyph
// for now — disambiguation comes from the inline subtype suffix
// (see subtypeSuffix) rather than a unique icon.
func kindIcon(kind string) string {
	switch kind {
	case "hub":
		return "⬡ "
	case "project":
		return "◈ "
	case "folder":
		return "  "
	case "design", "configured":
		return "  "
	case "drawing":
		return "  "
	case "schematic", "pcb", "ecad":
		return "  "
	default:
		return "  "
	}
}
