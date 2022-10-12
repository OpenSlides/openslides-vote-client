package main

import (
	"fmt"
	"os"

	"github.com/OpenSlides/openslides-performance/client"
	"github.com/OpenSlides/openslides-vote-client/tui"
	"github.com/alecthomas/kong"
)

func main() {
	kong.Parse(&cli, kong.UsageOnError())

	if err := tui.Run(cli.Config, cli.PollID); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

}

var cli struct {
	client.Config

	PollID int `arg:"" help:"ID of the poll."`
}
