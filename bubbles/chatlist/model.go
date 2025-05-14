package chatlist

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type ChatItem struct {
	ID   string
	Text string
}

func (ci ChatItem) FilterValue() string {
	return ci.Text
}

type Model struct {
	vp *viewport.Model
}

func New() *Model {
	return &Model{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m Model) View() string {
	return ""
}
