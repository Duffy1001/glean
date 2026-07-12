package main

import (
	"context"
	"fmt"
	"os"

	"github.com/duffy1001/glean/internal/app"
	"github.com/duffy1001/glean/internal/extract"
)

func main() {
	defer extract.ShutdownBackend()
	opts, err := app.ParseOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := app.Run(context.Background(), opts, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
