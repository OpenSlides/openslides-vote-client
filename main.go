package main

import (
	"fmt"
	"os"

	"github.com/OpenSlides/openslides-vote-client/client"
	"github.com/OpenSlides/openslides-vote-client/tui"
	"github.com/alecthomas/kong"
)

func main() {
	kong.Parse(&cli, kong.UsageOnError())

	if err := tui.Run(cli.Config, cli.PollID, cli.MainKey); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

}

var cli struct {
	client.Config

	PollID  int    `arg:"" help:"ID of the poll."`
	MainKey string `help:"Public main key from vote decrypt as base64." short:"k"`
}
