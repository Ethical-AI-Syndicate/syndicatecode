package main

import (
	"context"
	"log"
	"os"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/pkg/tui"
)

func main() {
	baseURL := "http://localhost:7777"
	if v := os.Getenv("SYNDICATE_CONTROLPLANE_URL"); v != "" {
		baseURL = v
	}

	client := tui.NewAPIClient(baseURL)
	app := tui.NewApp(client, os.Stdin, os.Stdout)

	if err := app.Run(context.Background()); err != nil {
		log.Fatalf("tui failed: %v", err)
	}
}
