package tui

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/OpenSlides/openslides-vote-client/client"
	tea "github.com/charmbracelet/bubbletea"
)

// Run runs the command.
func Run(cliCfg client.Config, pollID int, mainKey string) error {
	cli, err := client.New(cliCfg)
	if err != nil {
		return fmt.Errorf("initial http client: %w", err)
	}

	model, err := initialModel(pollID, mainKey, cli)
	if err != nil {
		return fmt.Errorf("initial model: %w", err)
	}

	p := tea.NewProgram(model)
	if err := p.Start(); err != nil {
		return fmt.Errorf("running bubble tea app: %w", err)
	}

	return nil
}

type model struct {
	pollID     int
	pubMainKey []byte

	ticks int
	err   error

	user         user
	poll         poll
	organization organization

	hasVoted bool
	ballot   ballot

	// Non model stuff
	client *client.Client
}

type user struct {
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Title     string `json:"title"`
}

func (u user) String() string {
	// Code from autoupdate projector.
	parts := func(sp ...string) []string {
		var full []string
		for _, s := range sp {
			if s == "" {
				continue
			}
			full = append(full, s)
		}
		return full
	}(u.FirstName, u.LastName)

	if len(parts) == 0 {
		parts = append(parts, u.Username)
	} else if u.Title != "" {
		parts = append([]string{u.Title}, parts...)
	}
	return strings.Join(parts, " ")
}

type poll struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	Type           string `json:"type"`
	Method         string `json:"pollmethod"`
	State          string `json:"state"`
	MinVotes       int    `json:"min_votes_amount"`
	MaxVotes       int    `json:"max_votes_amount"`
	MaxVotesOption int    `json:"max_votes_per_option"`
	GlobalYes      bool   `json:"global_yes"`
	GlobalNo       bool   `json:"global_no"`
	GlobalAbstain  bool   `json:"global_abstain"`
	OptionIDs      []int  `json:"option_ids"`
	CryptKey       []byte `json:"crypt_key"`
	CryptKeySig    []byte `json:"crypt_signature"`
	VotesRaw       string `json:"votes_raw"`
	VotesSignature []byte `json:"votes_signature"`
}

type organization struct {
	URL string `json:"url"`
}

func (o organization) Domain() (string, error) {
	parsed, err := url.Parse(o.URL)
	if err != nil {
		return "", fmt.Errorf("invalid url %s: %w", o.URL, err)
	}

	return parsed.Hostname(), nil
}

type ballot struct {
	optionID int
	selected int
	err      error
	sending  bool
	token    string

	debugVote string
}

func initialModel(pollID int, mainKey string, client *client.Client) (model, error) {
	var key []byte
	if len(mainKey) > 0 {
		var err error
		key, err = base64.StdEncoding.DecodeString(mainKey)
		if err != nil {
			return model{}, fmt.Errorf("decoding main key from base64: %w", err)
		}
	}

	return model{
		pollID:     pollID,
		pubMainKey: key,
		client:     client,
	}, nil
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tick, login(m.client))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "up":
			m.ballot.selected--
			return m, nil
		case "down":
			m.ballot.selected++
			return m, nil

		case "enter":
			m.ballot.sending = true
			m.ballot.err = nil
			voteValue, newBallot, err := createVote(m.poll, m.ballot, m.pubMainKey)
			if err != nil {
				m.err = fmt.Errorf("creating vote: %w", err)
				return m, nil
			}

			m.ballot = newBallot

			return m, vote(m.client, m.pollID, voteValue)
		}

	case msgTick:
		m.ticks++
		return m, tick

	case msgLogin:
		if err := msg.err; err != nil {
			m.err = fmt.Errorf("login: %w", err)
			return m, nil
		}

		cmdAU := autoupdateConnect(m.client, autoupdateRequest(m.client.UserID(), m.pollID))
		cmdVoted := haveIVoted(m.client, m.pollID)

		return m, tea.Batch(cmdAU, cmdVoted)

	case msgAutoupdate:
		if err := msg.err; err != nil {
			m.err = fmt.Errorf("autoupdate: %w", err)
			return m, nil
		}

		if err := parseKV("organization", 1, msg.value, &m.organization); err != nil {
			m.err = fmt.Errorf("parsing organization: %w", err)
			return m, nil
		}

		if err := parseKV("user", m.client.UserID(), msg.value, &m.user); err != nil {
			m.err = fmt.Errorf("parsing user: %w", err)
			return m, nil
		}

		oldState := m.poll.State

		if err := parseKV("poll", m.pollID, msg.value, &m.poll); err != nil {
			m.err = fmt.Errorf("parsing poll: %w", err)
			return m, nil
		}

		if len(m.poll.OptionIDs) == 0 {
			m.err = fmt.Errorf("Poll has no options")
			return m, nil
		}

		m.ballot.optionID = m.poll.OptionIDs[0]

		var cmds []tea.Cmd
		if oldState != m.poll.State {
			cmds = append(cmds, haveIVoted(m.client, m.pollID))
		}

		if !msg.finished {
			cmds = append(cmds, msg.next)
		}

		return m, tea.Batch(cmds...)

	case msgHaveIVoted:
		if err := msg.err; err != nil {
			m.err = fmt.Errorf("have i voted: %w", err)
			return m, nil
		}

		m.hasVoted = msg.voted
		return m, nil

	case msgVote:
		m.ballot.sending = false
		if err := msg.err; err != nil {
			m.ballot.err = fmt.Errorf("sending ballot: %w", err)
			return m, nil
		}

		m.hasVoted = true
		return m, nil
	}

	return m, nil
}

func createVote(poll poll, bt ballot, pubMainKey []byte) (string, ballot, error) {
	var v string
	switch bt.selected % 3 {
	case 0:
		v = "Y"
	case 1:
		v = "N"
	case 2:
		v = "A"
	}

	if poll.Type != "cryptographic" {
		value := fmt.Sprintf(`{"%d":"%s"}`, bt.optionID, v)
		return value, bt, nil
	}

	bt.token = createVoteToken()
	withtoken := fmt.Sprintf(`{"%d":"%s","token":"%s"}`, bt.optionID, v, bt.token)

	value, err := encryptVote(withtoken, pubMainKey, poll.CryptKey, poll.CryptKeySig)
	if err != nil {
		return "", ballot{}, fmt.Errorf("encrypting vote: %w", err)
	}
	bt.debugVote = value
	return value, bt, nil
}

func (m model) View() string {
	if m.err != nil {
		var errStatus client.HTTPStatusError
		if errors.As(m.err, &errStatus) && errStatus.StatusCode == 403 {
			var loginMsg struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(errStatus.Body, &loginMsg); err != nil {
				loginMsg.Message = string(errStatus.Body)
			}
			return fmt.Sprintf("Login impossible: %s", loginMsg.Message)
		}

		return fmt.Sprintf("Error: %v", m.err.Error())
	}

	userID := m.client.UserID()
	if userID == 0 {
		return fmt.Sprintf("Loggin in %s", viewProgress(m.ticks))
	}

	if m.user.Username == "" {
		return fmt.Sprintf("Logged in as user %d. Loading data %s", m.client.UserID(), viewProgress(m.ticks))
	}

	out := fmt.Sprintf("Hello %s!\n\n", m.user)

	if m.pubMainKey != nil {
		out += fmt.Sprintf("Please make sure the public main key is correct: %s\n\n", viewPubKey(m.pubMainKey))
	}

	out += m.viewPoll()

	return out
}

func viewPubKey(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

func viewProgress(ticks int) string {
	return strings.Repeat(".", (ticks%3)+1)
}

func (m model) viewPoll() string {
	if m.poll.ID == 0 {
		return fmt.Sprintf("The poll does currently not exist. Please wait %s", viewProgress(m.ticks))
	}

	content := new(bytes.Buffer)

	fmt.Fprintf(content, "Poll: %s (%s, %s)\n", m.poll.Title, m.poll.State, m.poll.Type)

	switch m.poll.State {
	case "started":
		if m.poll.Type == "cryptographic" {
			pollKeyValid := verify(m.pubMainKey, m.poll.CryptKey, m.poll.CryptKeySig)
			if !pollKeyValid {
				fmt.Fprintf(content, "Poll key is invalid")
				return content.String()
			}
			fmt.Fprintf(content, "Poll key is valid\n\n")
		}

		if m.ballot.err != nil {
			fmt.Fprintf(content, "Error: %v\n", m.ballot.err)
		}

		if m.hasVoted {
			fmt.Fprintf(content, "You already voted for poll %d\n", m.poll.ID)
			if m.ballot.debugVote != "" {
				fmt.Fprintf(content, "Your vote: %s", m.ballot.debugVote)
			}
			return content.String()
		}

		if m.ballot.sending {
			fmt.Fprintf(content, "Sending ballot %s\n", viewProgress(m.ticks))
		}

		if m.poll.Method != "YNA" {
			fmt.Fprintf(content, "Poll has type %s. This is not yet supported\n", m.poll.Type)
			return content.String()
		}

		if m.ballot.selected < 0 {
			m.ballot.selected += 3000
		}
		m.ballot.selected = m.ballot.selected % 3
		checked := map[bool]string{
			true:  "X",
			false: " ",
		}
		fmt.Fprintf(content, "[%s] Yes\n[%s] No\n[%s]Abstain\n", checked[m.ballot.selected == 0], checked[m.ballot.selected == 1], checked[m.ballot.selected == 2])

	case "published":
		domain, err := m.organization.Domain()
		if err != nil {
			fmt.Fprintf(content, "getting organization domain: %v", err)
		}

		if m.poll.Type == "cryptographic" {
			if err := verifyPollResults(m.pubMainKey, m.poll, domain, m.ballot.token); err != nil {
				fmt.Fprintf(content, "Poll results are invalid: %v\n\n", err)
			}
		}

		fmt.Fprintln(content, m.poll.VotesRaw)
	}

	return content.String()
}
