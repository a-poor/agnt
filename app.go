package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/urfave/cli/v3"
)

func makeApp() *cli.Command {
	return &cli.Command{
		Name:  "agnt",
		Usage: "...",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name: "init",
			},
		},
		Action: func(c context.Context, cmd *cli.Command) error {
			ctx, cancel := context.WithCancel(c)
			defer cancel()

			// Get the user's home directory
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}

			// Create the client...
			client, err := newClient(ctx, home)
			if err != nil {
				return err // TODO:
			}

			// TODO: Remove me...
			//
			// NOTE: This is just temporary
			if cmd.Bool("init") {
				ch, err := client.CreateChat("test")
				if err != nil {
					return err
				}
				fmt.Printf("Chat created with ID: %d\n", ch.ID)
				time.Sleep(time.Second)
			}

			// Create the agent...
			agent, err := newAgent(ctx, client)
			if err != nil {
				return err
			}

			// Create the model...
			m := newModel(ctx, client, agent)
			p := tea.NewProgram(m, tea.WithAltScreen())

			// TODO: Run the agent queue
			go func() {
				select {
				case <-ctx.Done():
					// Quit the TUI
					p.Quit()

					// Shut down the client
					if err := client.Close(); err != nil {
						panic(err)
					}
				case g := <-agent.gc:
					// Run the generation without blocking...
					fmt.Fprintln(os.Stderr, "Got generate msg in channel")
					if _, err := agent.generate(ctx, g.cid, func() {}); err != nil {
						panic(err)
					}
					fmt.Fprintln(os.Stderr, "Completed generate msg in channel")

					// Then tell the TUI to update
					m.Update(UpdateChatMsg{})
					fmt.Fprintln(os.Stderr, "Sent refresh msg")
				}
			}()

			// Run the model!
			if _, err := p.Run(); err != nil {
				return err
			}

			return nil
		},
	}
}
