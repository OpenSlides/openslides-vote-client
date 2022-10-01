package tui

import (
	"bufio"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseKV(t *testing.T) {
	var u user
	data := map[string]json.RawMessage{
		"user/1/username":   []byte(`"theHugo"`),
		"user/1/first_name": []byte(`"hugo"`),
		"user/1/last_name":  []byte(`"Man"`),
		"user/1/title":      []byte(`"The"`),
	}

	if err := parseKV("user", 1, data, &u); err != nil {
		t.Fatalf("parsing user: %v", err)
	}

	expect := user{"theHugo", "hugo", "Man", "The"}
	if u != expect {
		t.Errorf("Got `%v`, expected `%v`", u, expect)
	}
}

func TestAutoupdateMsg(t *testing.T) {
	data := `{"k1":"v1"}
	{"k2":"v2"}
	{"k1":"neu","k2":null}
	`
	msg := msgAutoupdate{
		scanner: bufio.NewScanner(strings.NewReader(data)),
	}

	msg = msg.next().(msgAutoupdate)
	got1 := msg.value
	expect1 := map[string]json.RawMessage{"k1": []byte(`"v1"`)}
	if !reflect.DeepEqual(got1, expect1) {
		t.Errorf("got1 %v, expected %v", got1, expect1)
	}

	msg = msg.next().(msgAutoupdate)
	got2 := msg.value
	expect2 := map[string]json.RawMessage{"k1": []byte(`"v1"`), "k2": []byte(`"v2"`)}
	if !reflect.DeepEqual(got2, expect2) {
		t.Errorf("got2 %v, expected %v", got2, expect2)
	}

	msg = msg.next().(msgAutoupdate)
	got3 := msg.value
	expect3 := map[string]json.RawMessage{"k1": []byte(`"neu"`)}
	if !reflect.DeepEqual(got3, expect3) {
		t.Errorf("got3 %v, expected %v", got3, expect3)
	}
}
