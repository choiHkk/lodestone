package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"lodestone/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "code similarity:", err)

		return 1
	}

	return 0
}
