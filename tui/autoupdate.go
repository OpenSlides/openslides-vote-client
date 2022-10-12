package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/OpenSlides/openslides-performance/client"
	tea "github.com/charmbracelet/bubbletea"
)

func autoupdateConnect(cli *client.Client, request string) tea.Cmd {
	return func() tea.Msg {
		url := "/system/autoupdate"

		req, err := http.NewRequest("GET", url, strings.NewReader(request))
		if err != nil {
			return msgAutoupdate{err: fmt.Errorf("creating request: %w", err)}
		}

		resp, err := cli.Do(req)
		if err != nil {
			return msgAutoupdate{err: fmt.Errorf("sending request: %w", err)}
		}

		return msgAutoupdate{scanner: bufio.NewScanner(resp.Body), closeBody: resp.Body}.next()
	}
}

type msgAutoupdate struct {
	err       error
	finished  bool
	value     map[string]json.RawMessage
	closeBody io.Closer

	scanner *bufio.Scanner
}

func (auMsg msgAutoupdate) next() tea.Msg {
	if auMsg.scanner == nil {
		return auMsg
	}

	if !auMsg.scanner.Scan() {
		auMsg.closeBody.Close()

		if err := auMsg.scanner.Err(); err != nil {
			return msgAutoupdate{err: fmt.Errorf("scanning next message: %w", err)}
		}
		return msgAutoupdate{finished: true, value: auMsg.value}
	}

	if err := json.Unmarshal(auMsg.scanner.Bytes(), &auMsg.value); err != nil {
		return msgAutoupdate{err: fmt.Errorf("decoding line: %w", err)}
	}

	for k, v := range auMsg.value {
		if v == nil || string(v) == "null" {
			delete(auMsg.value, k)
		}
	}

	return auMsg
}

func jsonTags(object any) []string {
	st := reflect.TypeOf(object)
	out := make([]string, 0, st.NumField())
	for i := 0; i < st.NumField(); i++ {
		field := st.Field(i)
		if alias, ok := field.Tag.Lookup("json"); ok {
			if alias != "" {
				out = append(out, alias)
			}
		}
	}
	return out
}

func autoupdateRequestPart(collection string, id int, object any) string {
	fields := jsonTags(object)
	withNull := make([]string, len(fields))
	for i, f := range fields {
		withNull[i] = fmt.Sprintf(`"%s":null`, f)
	}

	return fmt.Sprintf(
		`{"collection":"%s","ids":[%d],"fields":{%s}}`,
		collection,
		id,
		strings.Join(withNull, ","),
	)
}

func autoupdateRequest(userID int, pollID int) string {
	return "[" + strings.Join(
		[]string{
			autoupdateRequestPart("user", userID, user{}),
			autoupdateRequestPart("poll", pollID, poll{}),
			autoupdateRequestPart("organization", 1, organization{}),
		},
		",",
	) + "]"
}

func parseKV(collection string, id int, value map[string]json.RawMessage, object any) error {
	relevant := make(map[string]json.RawMessage)
	prefix := fmt.Sprintf("%s/%d/", collection, id)

	for k, v := range value {
		if strings.HasPrefix(k, prefix) {
			relevant[k[len(prefix):]] = v
		}
	}
	v, err := json.Marshal(relevant)
	if err != nil {
		return fmt.Errorf("encoding relevant keys: %w", err)
	}
	if err := json.Unmarshal(v, object); err != nil {
		return fmt.Errorf("decoding object: %w", err)
	}
	return nil
}
