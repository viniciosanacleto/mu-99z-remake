package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConvertsTheServerGroup(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"1.ConnectServer/ConnectServer.ini": "[ConnectServerInfo]\nConnectServerPortTCP = 44405\n",
		"1.ConnectServer/BlackList.txt":     "\"127.0.0.1\"\nend\n",
		"1.ConnectServer/ServerList.dat":    "0 \"Servidor 01\" \"192.168.1.1\" 55900 1\nend\n",
		"3.JoinServer/JoinServer.ini":       "[JoinServerInfo]\nMaxConnectionPerIP = 0\n",
		"2.DataServer/BadSyntax.txt":        "end\n",
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dst := t.TempDir()
	var out bytes.Buffer
	if err := run([]string{"-from", root, "-to", dst, "-only", "server"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "wrote server.toml\n" {
		t.Fatalf("output %q", out.String())
	}
	b, err := os.ReadFile(filepath.Join(dst, "server.toml"))
	if err != nil || !strings.Contains(string(b), `ServerName = "Servidor 01"`) {
		t.Fatalf("server.toml: %s %v", b, err)
	}
}

func TestRunErrors(t *testing.T) {
	var out bytes.Buffer
	if err := run(nil, &out); err == nil {
		t.Fatal("missing -from accepted")
	}
	if err := run([]string{"-from", t.TempDir(), "-only", "nope"}, &out); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("unknown group: %v", err)
	}
	if err := run([]string{"-from", t.TempDir(), "-to", t.TempDir(), "-only", "item"}, &out); err == nil {
		t.Fatal("missing source files accepted")
	}
}
