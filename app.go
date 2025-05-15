package main

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"
)

func makeApp() *cli.Command {
	return &cli.Command{
		Name:  "agnt",
		Usage: "...",
		Action: func(c context.Context, cmd *cli.Command) error {
			// Get the user's home directory
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}

			// Create the client...
			client, err := newClient(c, home)
			if err != nil {
				return err // TODO:
			}

			// Create the agent...
			agent, err := newAgent(c)
			if err != nil {
				return err
			}

			return nil
		},
	}
}
