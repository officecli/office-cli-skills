package main

import (
	"context"
	"fmt"
	"os"

	"github.com/officecli/officecli/internal/cli"
)

func main() {
	app := cli.NewApp(os.Stdout, os.Stderr, os.Stdin)
	if err := app.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
