package chatlist

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type Model struct {
	ta *textarea.Model
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

type ChatMsg struct {
	ChatID string // ID of chat thread in which to add the message
	Text   string //
}

func (m ChatMsg) AsMsg() tea.Msg {
	return m
}

type RefreshChatMsg struct {
	ChatID string
}

func (m RefreshChatMsg) AsMsg() tea.Msg {
	return m
}
