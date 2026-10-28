package main

import "testing"

func TestParseFlags(t *testing.T) {
	t.Setenv("MU_DATABASE_URL", "")
	t.Setenv("MU_EDITOR_PASSWORD", "")
	if _, err := parseFlags(nil); err == nil {
		t.Fatal("missing -db accepted")
	}
	c, err := parseFlags([]string{"-db", "postgres://x", "-password", "p"})
	if err != nil {
		t.Fatal(err)
	}
	if c.addr != "127.0.0.1:8080" || c.password != "p" || c.instance != "gameserver" || c.configDir != "config" {
		t.Fatalf("config = %+v", c)
	}
}
