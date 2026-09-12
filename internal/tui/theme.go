package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// initTheme drops every colour when NO_COLOR is set.
//
// lipgloss already adapts to light and dark backgrounds, but a user who has
// asked for no colour at all - a pipe, a screen reader, a terminal without
// them - is asking for something the adaptive palette cannot express.
func initTheme() {
	if os.Getenv("NO_COLOR") == "" {
		return
	}
	plain := lipgloss.NewStyle()
	bold := lipgloss.NewStyle().Bold(true)
	styTitle, styDate = bold, plain
	styTabActive, styTabInactive = bold.Underline(true), plain
	styGroup, styMuted = bold, plain
	styDoing, styBlocked, styDanger = plain, plain, plain
	styDoneRow = lipgloss.NewStyle().Strikethrough(true)
	stySelected = bold
	styHelp, styStatus, styErr, styPrompt = plain, plain, plain, bold
	styRule = plain
}

// Colors are adaptive so the same binary is legible on light and dark
// terminals without a theme setting.
var (
	colFg      = lipgloss.AdaptiveColor{Light: "236", Dark: "252"}
	colMuted   = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
	colAccent  = lipgloss.AdaptiveColor{Light: "26", Dark: "39"}
	colDoing   = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}
	colBlocked = lipgloss.AdaptiveColor{Light: "130", Dark: "214"}
	colDanger  = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	colDone    = lipgloss.AdaptiveColor{Light: "245", Dark: "242"}
)

var (
	styTitle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styDate  = lipgloss.NewStyle().Foreground(colMuted)

	styTabActive   = lipgloss.NewStyle().Bold(true).Foreground(colAccent).Underline(true)
	styTabInactive = lipgloss.NewStyle().Foreground(colMuted)

	styGroup    = lipgloss.NewStyle().Bold(true).Foreground(colFg)
	styMuted    = lipgloss.NewStyle().Foreground(colMuted)
	styDoing    = lipgloss.NewStyle().Foreground(colDoing)
	styBlocked  = lipgloss.NewStyle().Foreground(colBlocked)
	styDanger   = lipgloss.NewStyle().Foreground(colDanger)
	styDoneRow  = lipgloss.NewStyle().Foreground(colDone).Strikethrough(true)
	stySelected = lipgloss.NewStyle().Bold(true).Foreground(colAccent)

	styHelp   = lipgloss.NewStyle().Foreground(colMuted)
	styStatus = lipgloss.NewStyle().Foreground(colAccent)
	styErr    = lipgloss.NewStyle().Foreground(colDanger)
	styPrompt = lipgloss.NewStyle().Bold(true).Foreground(colAccent)

	styRule = lipgloss.NewStyle().Foreground(colMuted)
)
