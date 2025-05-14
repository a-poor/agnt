package main

import tea "github.com/charmbracelet/bubbletea"

type chat struct{}

func (c chat) Init() tea.Cmd {
	return nil
}

func (c chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return c, nil
}

func (c chat) View() string {
	return ""
}
