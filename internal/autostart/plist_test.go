package autostart

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// plistStrings returns the text of every <string> element, in order, and
// fails if the document is not well-formed XML.
func plistStrings(t *testing.T, doc string) []string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(doc))
	var out []string
	var cur string
	inString := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("invalid XML: %v\n%s", err, doc)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if v.Name.Local == "string" {
				inString, cur = true, ""
			}
		case xml.CharData:
			if inString {
				cur += string(v)
			}
		case xml.EndElement:
			if v.Name.Local == "string" {
				out = append(out, cur)
				inString = false
			}
		}
	}
}

func TestLaunchAgentPlist(t *testing.T) {
	exe := `/Applications/VPN & "Co" <beta>/it's/vpn_specify`
	doc := launchAgentPlist("io.github.d3t3f.vpn_specify", []string{exe, Arg})
	got := plistStrings(t, doc)
	want := []string{"io.github.d3t3f.vpn_specify", exe, "autostart"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("strings = %q; want %q", got, want)
	}
	if !strings.Contains(doc, "<key>RunAtLoad</key>\n\t<true/>") {
		t.Errorf("RunAtLoad is not true:\n%s", doc)
	}
	if strings.Contains(doc, `VPN & "Co"`) {
		t.Errorf("values are not escaped:\n%s", doc)
	}
}
