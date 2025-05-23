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

			// Run the agent worker goroutine
			go func() {
				for {
					select {
					case <-ctx.Done():
						// Quit the TUI
						p.Quit()

						// Shut down the client
						if err := client.Close(); err != nil {
							panic(err)
						}
						return
					case req := <-agent.gc:
						// Update chat state to running
						if err := client.UpdateChatState(req.ChatID, "running"); err != nil {
							p.Send(GenerateResponse{ChatID: req.ChatID, Error: err})
							continue
						}

						// Run the generation
						fmt.Fprintln(os.Stderr, "Got generate request for chat", req.ChatID)
						_, err := agent.generate(ctx, req.ChatID, func() {
							// Send intermediate update
							p.Send(UpdateChatMsg{})
						})
						
						// Update chat state back to idle
						if stateErr := client.UpdateChatState(req.ChatID, "idle"); stateErr != nil {
							if err == nil {
								err = stateErr
							}
						}

						// Send completion response
						p.Send(GenerateResponse{ChatID: req.ChatID, Error: err})
						fmt.Fprintln(os.Stderr, "Completed generate for chat", req.ChatID)
					}
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
