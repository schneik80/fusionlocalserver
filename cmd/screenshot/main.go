// cmd/screenshot/main.go
//
// Renders a synthetic fusionlocalserver screen with sample data.
// Pipe through freeze to produce a PNG:
//
//	go run ./cmd/screenshot | freeze -o docs/screenshot.png
package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Force true-color output even when stdout is not a TTY (e.g. piped to freeze).
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
}

// ---------------------------------------------------------------------------
// Theme (Rust — default)
// ---------------------------------------------------------------------------

var (
	accent      = lipgloss.Color("#C05A1F")
	subtle      = lipgloss.Color("#555555")
	muted       = lipgloss.Color("#888888")
	fg          = lipgloss.Color("#FFFFFF")
	detailKey   = lipgloss.Color("#888888")
	containerFg = lipgloss.Color("#89B4D4")
	documentFg  = lipgloss.Color("#FFFFFF")

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			Padding(0, 1)

	styleStatus = lipgloss.NewStyle().
			Foreground(muted).
			Italic(true)

	styleColumnActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(accent).
				Padding(0, 1)

	styleColumnInactive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(subtle).
				Padding(0, 1)

	styleColumnTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(accent).
				MarginBottom(1)

	styleSelected = lipgloss.NewStyle().
			Bold(true).
			Foreground(fg).
			Background(accent).
			Padding(0, 1)

	styleNormal = lipgloss.NewStyle().
			Foreground(fg).
			Padding(0, 1)

	styleInactiveSelected = lipgloss.NewStyle().
				Foreground(accent).
				Padding(0, 1)

	styleContainer = lipgloss.NewStyle().
			Foreground(containerFg).
			Padding(0, 1)

	styleDocument = lipgloss.NewStyle().
			Foreground(documentFg).
			Padding(0, 1)

	styleDim = lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1)

	styleFooter = lipgloss.NewStyle().
			Foreground(muted).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(subtle).
			Padding(0, 1)

	styleDetailKey = lipgloss.NewStyle().Foreground(detailKey)
	styleDetailVal = lipgloss.NewStyle().Foreground(fg)

	styleMilestone = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true)
)

// ---------------------------------------------------------------------------
// Sample data
// ---------------------------------------------------------------------------

type col struct {
	title    string
	items    []string // rendered label strings
	selected int      // selected index (0-based)
	active   bool
}

func renderColumn(c col, width, height int) string {
	title := styleColumnTitle.Render(c.title)

	var lines []string
	for i, item := range c.items {
		switch {
		case i == c.selected && c.active:
			lines = append(lines, styleSelected.Width(width).Render(item))
		case i == c.selected && !c.active:
			lines = append(lines, styleInactiveSelected.Width(width).Render(item))
		case isContainer(item):
			lines = append(lines, styleContainer.Width(width).Render(item))
		default:
			lines = append(lines, styleDocument.Width(width).Render(item))
		}
	}

	// pad to height
	inner := height - 4 // title + border + margin
	for len(lines) < inner {
		lines = append(lines, styleNormal.Width(width).Render(""))
	}

	body := title + "\n" + strings.Join(lines, "\n")

	if c.active {
		return styleColumnActive.Width(width).Height(height - 2).Render(body)
	}
	return styleColumnInactive.Width(width).Height(height - 2).Render(body)
}

func isContainer(label string) bool {
	return strings.HasPrefix(label, "⬡") ||
		strings.HasPrefix(label, "◈") ||
		strings.HasSuffix(strings.TrimSpace(label), "/")
}

func renderDetails(width, height int) string {
	kw := 10 // key column width

	line := func(k, v string) string {
		if k == "" && v == "" {
			return ""
		}
		key := styleDetailKey.Width(kw).Render(k)
		val := styleDetailVal.Render(v)
		return key + " " + val
	}

	sep := strings.Repeat("─", width-2)

	rows := []string{
		styleColumnTitle.Render("Wing Assembly"),
		"",
		line("Size", "47.2 MB"),
		line("Version", "v12"),
		line("Type", "DesignItem"),
		line("Ext", ".f3d"),
		"",
		sep,
		styleDetailKey.Render("Created"),
		"  " + styleDetailVal.Render("Feb 14 2026"),
		"  " + styleDetailVal.Render("Priya Nair"),
		"",
		styleDetailKey.Render("Modified"),
		"  " + styleDetailVal.Render("Mar 29 2026"),
		"  " + styleDetailVal.Render("Tom Bachmann"),
		"",
		sep,
		styleDetailKey.Render("Component"),
		line("Part No.", "FAL-WING-001"),
		line("Desc", "Main wing assy"),
		line("Material", "Carbon Fiber"),
		styleMilestone.Render("  ★ Milestone"),
		"",
		sep,
		styleDetailKey.Render("Versions"),
		line("v12", "Mar 29  Tom Bachmann"),
		"    " + styleDim.Render("Updated spar thickness"),
		line("v11", "Mar 22  Priya Nair"),
		"    " + styleDim.Render("Aerodynamic revision"),
		line("v10", "Mar 10  Tom Bachmann"),
		"    " + styleDim.Render("Material swap: CFRP"),
		line("v9", "Feb 28  Priya Nair"),
		"    " + styleDim.Render("LE radius update"),
		line("v8", "Feb 21  Tom Bachmann"),
		"    " + styleDim.Render(""),
		line("v7", "Feb 14  Priya Nair"),
		"    " + styleDim.Render("Initial commit"),
	}

	// pad to fill height
	for len(rows) < height-4 {
		rows = append(rows, "")
	}

	body := strings.Join(rows, "\n")
	return styleColumnInactive.Width(width).Height(height - 2).Render(body)
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	const (
		totalWidth  = 190
		totalHeight = 42
	)

	// Column widths: details = 40%, nav = 60% split 3 ways
	detW := (totalWidth * 2) / 5        // ≈ 84
	navW := totalWidth - detW           // ≈ 126
	col0W := navW / 3                   // ≈ 42
	col1W := navW / 3                   // ≈ 42
	col2W := navW - col0W - col1W       // ≈ 42
	colH := totalHeight - 3             // subtract header + footer rows

	// ── Column 0: Hubs ────────────────────────────────────────────────────
	hubs := col{
		title: "Hubs",
		items: []string{
			"⬡ Acme Engineering",
			"⬡ Partner Portal",
		},
		selected: 0,
		active:   false,
	}

	// ── Column 1: Projects ────────────────────────────────────────────────
	projects := col{
		title: "Projects",
		items: []string{
			"◈ Falcon UAV",
			"◈ Osprey Drone",
			"◈ Component Library",
			"◈ Tooling & Fixtures",
			"◈ Archive 2024",
		},
		selected: 0,
		active:   false,
	}

	// ── Column 2: Contents (active) ───────────────────────────────────────
	contents := col{
		title: "CAD  ›  Falcon UAV",
		items: []string{
			"  Drawings/",
			"  Simulation/",
			"  Manufacturing/",
			"  Wing Assembly.f3d",
			"  Fuselage.f3d",
			"  Landing Gear.f3d",
			"  Avionics Bay.f3d",
			"  Propulsion Unit.f3d",
			"  Full Assembly.f3d",
		},
		selected: 3, // Wing Assembly selected
		active:   true,
	}

	// ── Render each column ────────────────────────────────────────────────
	c0 := renderColumn(hubs, col0W, colH)
	c1 := renderColumn(projects, col1W, colH)
	c2 := renderColumn(contents, col2W, colH)
	det := renderDetails(detW, colH)

	browser := lipgloss.JoinHorizontal(lipgloss.Top, c0, c1, c2, det)

	// ── Header ────────────────────────────────────────────────────────────
	title := styleHeader.Render("fusionlocalserver")
	status := styleStatus.Render("Acme Engineering  ›  Falcon UAV  ›  CAD")
	headerGap := totalWidth - lipgloss.Width(title) - lipgloss.Width(status) - 2
	if headerGap < 1 {
		headerGap = 1
	}
	header := title + strings.Repeat(" ", headerGap) + status

	// ── Footer ────────────────────────────────────────────────────────────
	footer := styleFooter.Width(totalWidth - 2).Render(
		"[↑↓/jk] move  [←→/hl] navigate  [d] details  [t] theme  [a] about  [q] quit",
	)

	// ── Compose ───────────────────────────────────────────────────────────
	screen := lipgloss.JoinVertical(lipgloss.Left,
		header,
		browser,
		footer,
	)

	fmt.Println(screen)
}
