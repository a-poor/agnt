package main

import (
	"context"
	"os"
)

func main() {
	ctx := context.Background()

	// Make the CLI app
	app := makeApp()

	// Run it
	if err := app.Run(ctx, os.Args); err != nil {
		panic(err)
	}

}
