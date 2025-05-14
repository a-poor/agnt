package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

var _ tea.Model = (*model)(nil)

type model struct{}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m *model) View() string {
	return ""
}
