package tui

import "github.com/charmbracelet/lipgloss"

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
