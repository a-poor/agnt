package main

import (
	"context"
	"os"
)

func main() {
	ctx := context.Background()
	if err := makeApp().Run(ctx, os.Args); err != nil {
		panic(err)
	}

}
