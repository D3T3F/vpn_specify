//go:build windows

package autostart

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestTaskXML(t *testing.T) {
	x := taskXML(`PC\Fulano & Cia`, `C:\Program Files\vpn_specify\vpn_specify.exe`)
	var v struct {
		Triggers struct {
			Logon struct {
				UserId string `xml:"UserId"`
				Delay  string `xml:"Delay"`
			} `xml:"LogonTrigger"`
		} `xml:"Triggers"`
		Principals struct {
			P struct {
				RunLevel  string `xml:"RunLevel"`
				LogonType string `xml:"LogonType"`
			} `xml:"Principal"`
		} `xml:"Principals"`
		Settings struct {
			Limit string `xml:"ExecutionTimeLimit"`
		} `xml:"Settings"`
		Exec struct {
			Command string `xml:"Command"`
			Args    string `xml:"Arguments"`
		} `xml:"Actions>Exec"`
	}
	// o encoding declarado é UTF-16 (o arquivo é gravado assim); aqui lemos a string
	d := xml.NewDecoder(strings.NewReader(strings.Replace(x, `encoding="UTF-16"`, `encoding="UTF-8"`, 1)))
	if err := d.Decode(&v); err != nil {
		t.Fatalf("XML inválido: %v\n%s", err, x)
	}
	if v.Triggers.Logon.UserId != `PC\Fulano & Cia` || v.Principals.P.RunLevel != "HighestAvailable" ||
		v.Principals.P.LogonType != "InteractiveToken" || v.Settings.Limit != "PT0S" ||
		v.Exec.Command != `"C:\Program Files\vpn_specify\vpn_specify.exe"` || v.Exec.Args != "autostart" {
		t.Errorf("tarefa errada: %+v", v)
	}
}
