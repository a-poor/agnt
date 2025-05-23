package main

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

var _ tea.Model = (*model)(nil)

type model struct {
	c    *client // Data client
	a    *agent  // LLM agent
	w, h int     // Track the size of the window

	chatId int

	ctx   context.Context
	focus string

	vp   *viewport.Model
	ta   *textarea.Model
	hist []Message
}

func newModel(ctx context.Context, c *client, a *agent) *model {
	// Set a default size (this will be updated quickly)
	w, h := 80, 24

	// Create a textarea
	ta := textarea.New()
	ta.SetHeight(1)
	ta.SetWidth(w)
	ta.Focus()

	// Create the viewport
	vp := viewport.New(w, h-ta.Height())

	// Load the chat history
	hist, err := c.ListMessages(1)
	if err != nil {
		panic(err)
	}

	// Combine and return
	return &model{
		c:      c,
		a:      a,
		chatId: 1,
		w:      w,
		h:      h,
		ctx:    ctx,
		focus:  "textarea",
		vp:     &vp,
		ta:     &ta,
		hist:   hist,
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			return UpdateChatMsg{}
		},
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Update the tracked size
		m.w, m.h = msg.Width, msg.Height

		// Update the textarea size
		m.ta.SetWidth(msg.Width)
		m.ta.SetHeight(min(msg.Height, m.ta.Height()))

		// Set the viewport size
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - m.ta.Height()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.focus == "textarea" {
				return m, tea.Batch(func() tea.Msg {
					return SetFocusMsg{focus: "viewport"}
				})
			} else {
				return m, tea.Batch(func() tea.Msg {
					return SetFocusMsg{focus: "textarea"}
				})
			}
		case "enter":
			if m.focus == "textarea" {
				m.ta.Blur()
				m.focus = "viewport"
				return m, func() tea.Msg {
					return SendMessageMsg{m.ta.Value()}
				}
			}
			if m.focus == "viewport" {
				vp, cmd := m.vp.Update(msg)
				m.vp = &vp
				return m, cmd
			}
		default:
			if m.focus == "textarea" {
				ta, cmd := m.ta.Update(msg)
				m.ta = &ta
				return m, cmd
			}
			if m.focus == "viewport" {
				vp, cmd := m.vp.Update(msg)
				m.vp = &vp
				return m, cmd
			}
		}
	case SetFocusMsg:
		switch msg.focus {
		case "textarea":
			m.focus = "textarea"
			m.ta.Focus()
		case "viewport":
			m.focus = "viewport"
			m.ta.Blur()
		}
	case SendMessageMsg:
		// Check if chat is already running
		chat, err := m.c.GetChat(m.chatId)
		if err != nil {
			panic(err)
		}
		if chat.State == "running" {
			// Ignore message if chat is already generating
			return m, nil
		}
		
		// Add the message to the database
		if _, err := m.c.CreateMessage(Message{
			ChatID:  m.chatId,
			MType:   "user",
			UserMsg: &struct{ Text string }{Text: msg.text},
		}); err != nil {
			panic(err)
		}
		m.ta.SetValue("")
		return m, tea.Batch(
			func() tea.Msg { return UpdateChatMsg{} },
			func() tea.Msg { return GenerateMsg{} },
			func() tea.Msg { return SetFocusMsg{focus: "textarea"} },
		)
	case GenerateMsg:
		// Send generation request through channel
		m.a.gc <- GenerateRequest{ChatID: m.chatId}
		return m, nil
	case UpdateChatMsg:
		// Get the history for the chat and store it
		hist, err := m.c.ListMessages(m.chatId)
		if err != nil {
			panic(err)
		}
		m.hist = hist

		// Update the viewport content
		m.updteVP()

		// Was the last message a tool call? Then keep going.
		if n := len(hist); n > 0 && hist[n-1].MType == "tool" {
			return m, tea.Batch(func() tea.Msg { return GenerateMsg{} })
		}
		return m, nil
	case GenerateResponse:
		if msg.Error != nil {
			// TODO: Handle error more gracefully
			panic(msg.Error)
		}
		// Refresh the chat to show the completed response
		return m, func() tea.Msg { return UpdateChatMsg{} }
	}
	return m, nil
}

func (m *model) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.vp.View(),
		m.ta.View(),
	)
}

func (m *model) updteVP() {
	var parts []string
	for _, msg := range m.hist {
		switch msg.MType {
		case "user":
			parts = append(parts, lipgloss.JoinHorizontal(
				lipgloss.Top,
				"👨‍💻: ",
				wordwrap.String(msg.UserMsg.Text, m.w-4),
			))
		case "agent":
			parts = append(parts, lipgloss.JoinHorizontal(
				lipgloss.Top,
				"🤖: ",
				wordwrap.String(msg.AgentMsg.Text, m.w-4),
			))
		case "tool":
			parts = append(parts, fmt.Sprintf(
				"🛠️: %s",
				lipgloss.
					NewStyle().
					Foreground(lipgloss.Color("#AAAFBE")).
					Render("Calling "+msg.ToolMsg.ToolName+"()..."),
			))
		default:
			panic(fmt.Sprintf("unknown message type %q", msg.MType))
		}
	}

	// Generate the text
	s := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Update the viewport
	m.vp.SetContent(s)
}

type SendMessageMsg struct {
	text string
}

type SetFocusMsg struct {
	focus string
}

type GenerateMsg struct{}

type UpdateChatMsg struct{}
