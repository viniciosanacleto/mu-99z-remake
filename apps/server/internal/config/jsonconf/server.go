package jsonconf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// ServerFile is config/server.json: the files of the original
// ConnectServer, JoinServer and DataServer folders.
type ServerFile struct {
	ConnectServer *Settings `json:"ConnectServer"` // 1.ConnectServer/ConnectServer.ini
	ServerList    *Table    `json:"ServerList"`    // 1.ConnectServer/ServerList.dat
	BlackList     *Table    `json:"BlackList"`     // 1.ConnectServer/BlackList.txt
	JoinServer    *Settings `json:"JoinServer"`    // 3.JoinServer/JoinServer.ini
	BadSyntax     *Table    `json:"BadSyntax"`     // 2.DataServer/BadSyntax.txt
}

// LoadServer reads config/server.json. Missing members are empty.
func LoadServer(path string) (*ServerFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f := &ServerFile{}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.ConnectServer == nil {
		f.ConnectServer = &Settings{}
	}
	if f.JoinServer == nil {
		f.JoinServer = &Settings{}
	}
	for _, t := range []**Table{&f.ServerList, &f.BlackList, &f.BadSyntax} {
		if *t == nil {
			*t = &Table{Sections: []Section{{}}}
		}
	}
	return f, nil
}

// WriteServer writes config/server.json.
func WriteServer(path string, f *ServerFile) error {
	var b bytes.Buffer
	b.WriteString("{")
	first := true
	member := func(name string) {
		if !first {
			b.WriteString(",")
		}
		first = false
		b.WriteString("\n  " + quote(name) + ": ")
	}
	for _, m := range []struct {
		name string
		s    *Settings
		t    *Table
	}{
		{"ConnectServer", f.ConnectServer, nil},
		{"ServerList", nil, f.ServerList},
		{"BlackList", nil, f.BlackList},
		{"JoinServer", f.JoinServer, nil},
		{"BadSyntax", nil, f.BadSyntax},
	} {
		switch {
		case m.s != nil:
			member(m.name)
			writeSettings(&b, m.s, "  ")
		case m.t != nil:
			member(m.name)
			writeTable(&b, m.t, "  ")
		}
	}
	b.WriteString("\n}\n")
	return writeFile(path, b.Bytes())
}
