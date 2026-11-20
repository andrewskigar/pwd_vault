// Command save_pwd is the composition root: it wires real process resources
// into the CLI adapter and nothing else.
package main

import (
	"context"
	"crypto/rand"
	"os"
	"os/signal"

	"save_pwd/internal/adapters/cli"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run())
}

// run returns the exit code so the deferred signal release happens before
// os.Exit, which would skip it.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return cli.Execute(ctx, os.Args[1:], cli.Deps{
		Streams: cli.Streams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
		Random:  rand.Reader,
		Version: version,
	})
}
