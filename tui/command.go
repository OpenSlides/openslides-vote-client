package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/OpenSlides/openslides-vote-client/client"
	tea "github.com/charmbracelet/bubbletea"
)

func tick() tea.Msg {
	time.Sleep(time.Second)
	return msgTick{}
}

type msgTick struct{}

func login(cli *client.Client) tea.Cmd {
	return func() tea.Msg {
		err := cli.Login(context.Background())
		return msgLogin{err}
	}
}

type msgLogin struct {
	err error
}

func vote(cli *client.Client, pollID int, ballot string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("/system/vote?id=%d", pollID)

		content := fmt.Sprintf(`{"value":%s}`, ballot)
		req, err := http.NewRequest("POST", url, strings.NewReader(content))
		if err != nil {
			return msgVote{err: fmt.Errorf("creating request: %w", err)}
		}

		resp, err := cli.Do(req)
		if err != nil {
			return msgVote{err: fmt.Errorf("sending request: %w", err)}
		}

		io.ReadAll(resp.Body)
		resp.Body.Close()

		return msgVote{}
	}
}

type msgVote struct {
	err error
}

func haveIVoted(cli *client.Client, pollID int) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("/system/vote/voted?ids=%d", pollID)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return msgHaveIVoted{err: fmt.Errorf("creating request: %w", err)}
		}

		resp, err := cli.Do(req)
		if err != nil {
			return msgHaveIVoted{err: fmt.Errorf("sending request: %w", err)}
		}
		defer resp.Body.Close()

		var content map[int]bool
		if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
			return msgHaveIVoted{err: fmt.Errorf("decoding response body: %w", err)}
		}

		return msgHaveIVoted{voted: content[pollID]}

	}
}

type msgHaveIVoted struct {
	err   error
	voted bool
}
