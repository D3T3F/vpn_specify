package mobile

import (
	"encoding/json"
	"strings"
	"testing"
)

func call(t *testing.T, req Request) (result json.RawMessage, errMsg string) {
	t.Helper()
	b, _ := json.Marshal(req)
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(Call(b), &r); err != nil {
		t.Fatal(err)
	}
	return r.Result, r.Error
}

func TestLanguages(t *testing.T) {
	res, e := call(t, Request{Method: "setLanguage", Text: "pt-BR"})
	if e != "" {
		t.Fatal(e)
	}
	var cat map[string]string
	if err := json.Unmarshal(res, &cat); err != nil || cat["Disconnect"] == "" {
		t.Fatalf("catalog: %v, %d entries", err, len(cat))
	}
	// errors coming from Go follow the same language
	_, e = call(t, Request{Method: "plan", Config: "[Interface]\n"})
	call(t, Request{Method: "setLanguage", Text: "en"})
	if e == "" || strings.Contains(e, "PrivateKey missing") {
		t.Errorf("error not translated: %q", e)
	}
	res, _ = call(t, Request{Method: "matchLanguage", Text: "pt-PT"})
	if string(res) != `"pt-BR"` {
		t.Errorf("match: %s", res)
	}
}

func TestBadRequests(t *testing.T) {
	if _, e := call(t, Request{Method: "nope"}); e == "" {
		t.Error("unknown method must fail")
	}
	if !strings.Contains(string(Call([]byte("not json"))), "error") {
		t.Error("bad JSON must fail")
	}
	// with nothing running these are harmless
	for _, m := range []string{"stop", "rebind", "kick", "stats", "log"} {
		if _, e := call(t, Request{Method: m}); e != "" {
			t.Errorf("%s: %s", m, e)
		}
	}
}
