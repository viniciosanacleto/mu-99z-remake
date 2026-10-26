package servercfg

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadShipped(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	f, err := Load(filepath.Join(filepath.Dir(file), "..", "..", "..", "config", "server.toml"))
	if err != nil {
		t.Fatal(err)
	}
	c := f.ConnectServer
	if c.ConnectServerPortTCP != 44405 || c.MaxConnectionIdle != 60 || c.MaxConnectionPerIP != 1 ||
		c.MaxPacketPerSecond != 10 || len(c.BlackList) != 1 || c.BlackList[0] != "127.0.0.1" || len(c.ServerList) != 2 {
		t.Fatalf("connect = %+v", c)
	}
	if f.JoinServer.MaxConnectionPerIP != 0 || f.JoinServer.JoinServerODBC != "MuOnline" {
		t.Fatalf("join = %+v", f.JoinServer)
	}
}

func TestLoadDefaultsAndStrict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.toml")
	os.WriteFile(path, []byte("[ConnectServer]\nMaxConnectionPerIP = 2\n"), 0o644)
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.ConnectServer.ConnectServerPortTCP != 44405 || f.ConnectServer.MaxConnectionIdle != 60 || f.ConnectServer.MaxConnectionPerIP != 2 {
		t.Fatalf("defaults = %+v", f.ConnectServer)
	}
	os.WriteFile(path, []byte("[ConnectServer]\nMaxConectionPerIP = 2\n"), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("unknown key accepted")
	}
}
