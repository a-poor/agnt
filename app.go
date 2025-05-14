package main

import (
	"context"

	"github.com/urfave/cli/v3"
)

func makeApp() *cli.Command {
	return &cli.Command{
		Name:  "agnt",
		Usage: "...",
		Action: func(c context.Context, cmd *cli.Command) error {
			return nil
		},
	}
}
