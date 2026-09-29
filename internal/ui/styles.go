package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
)

var (
	colorAccent  = lipgloss.Color("#8ec07c")
	colorBorder  = lipgloss.Color("#504945")
	colorMuted   = lipgloss.Color("#928374")
	colorText    = lipgloss.Color("#ebdbb2")
	colorSelBg   = lipgloss.Color("#3c3836")
	colorBadge   = lipgloss.Color("#fabd2f")
	colorHeading = lipgloss.Color("#83a598")

	styleMuted      = lipgloss.NewStyle().Foreground(colorMuted)
	styleText       = lipgloss.NewStyle().Foreground(colorText)
	styleBold       = lipgloss.NewStyle().Foreground(colorText).Bold(true)
	styleBadge      = lipgloss.NewStyle().Foreground(colorBadge).Bold(true)
	styleHeading    = lipgloss.NewStyle().Foreground(colorHeading).Bold(true)
	styleSelected   = lipgloss.NewStyle().Background(colorSelBg).Foreground(colorAccent).Bold(true)
	styleSelDim     = lipgloss.NewStyle().Background(colorSelBg).Foreground(colorText)
	styleHeader     = lipgloss.NewStyle().Foreground(colorText)
	styleBrand      = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleNotice     = lipgloss.NewStyle().Foreground(colorBadge)
	styleFlash      = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleDisclaimer = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	styleError      = lipgloss.NewStyle().Foreground(lipgloss.Color("#fb4934")).Bold(true)
)

// categoryColors must cover every model.Category (enforced by a test over
// model.Categories()); categoryStyle falls back to plain text regardless.
var categoryColors = map[model.Category]color.Color{
	model.CategoryTech:      lipgloss.Color("#83a598"),
	model.CategoryMarket:    lipgloss.Color("#d3869b"),
	model.CategoryNews:      lipgloss.Color("#ebdbb2"),
	model.CategoryOpinion:   lipgloss.Color("#fe8019"),
	model.CategoryEvent:     lipgloss.Color("#fabd2f"),
	model.CategoryValuation: lipgloss.Color("#b8bb26"),
	model.CategoryOther:     lipgloss.Color("#928374"),
}

// stanceColors must cover every model.Stance (enforced by a test).
var stanceColors = map[model.Stance]color.Color{
	model.StanceLong:  lipgloss.Color("#b8bb26"),
	model.StanceShort: lipgloss.Color("#fb4934"),
	model.StanceNone:  lipgloss.Color("#928374"),
}

func categoryStyle(c model.Category) lipgloss.Style {
	col, ok := categoryColors[c]
	if !ok {
		col = colorText
	}
	return lipgloss.NewStyle().Foreground(col)
}

func stanceStyle(s model.Stance) lipgloss.Style {
	col, ok := stanceColors[s]
	if !ok {
		col = colorText
	}
	return lipgloss.NewStyle().Foreground(col).Bold(true)
}
