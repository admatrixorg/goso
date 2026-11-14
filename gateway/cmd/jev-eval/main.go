// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command jev-eval scores the embedded Minh An (demo) labels with Jev.
// With no GOSO_JEV_API_KEY it prints a skip line and exits 0.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mqglobal/goso/gateway/internal/jev"
)

func main() {
	rep, err := jev.RunEval(context.Background())
	fmt.Println(rep.String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "jev-eval: %s\n", err.Error())
		os.Exit(1)
	}
}
