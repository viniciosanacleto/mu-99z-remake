package editor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func doJSON(t *testing.T, srv *httptest.Server, method, path string, body any, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, srv.URL+path, rd)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return resp.StatusCode
}

func TestHTTPAPI(t *testing.T) {
	f := newFixture(t)
	srv := httptest.NewServer(Handler(f.ed, "", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(page), "<title>MU Editor</title>") {
		t.Fatalf("index = %d", resp.StatusCode)
	}

	var cat struct {
		Categories []string      `json:"categories"`
		Items      []CatalogItem `json:"items"`
	}
	if code := doJSON(t, srv, "GET", "/api/items", nil, &cat); code != 200 || len(cat.Categories) != 16 ||
		len(cat.Items) != len(f.cat.Items) {
		t.Fatalf("items = %d, %d categories, %d items", code, len(cat.Categories), len(cat.Items))
	}

	var rows []map[string]any
	if code := doJSON(t, srv, "GET", "/api/characters?q=arth", nil, &rows); code != 200 || len(rows) != 1 ||
		rows[0]["name"] != "Heroi" || rows[0]["login"] != "arthur" || rows[0]["online"] != false {
		t.Fatalf("search = %d %v", code, rows)
	}

	kris := byName(t, f.cat, 0, "Kris")
	var added map[string]int
	if code := doJSON(t, srv, "POST", "/api/characters/Heroi/items",
		map[string]any{"container": "vault", "index": kris.Index, "level": 7, "luck": true}, &added); code != 201 || added["slot"] != 0 {
		t.Fatalf("add = %d %v", code, added)
	}
	var v CharacterView
	if code := doJSON(t, srv, "GET", "/api/characters/heroi", nil, &v); code != 200 || len(v.Vault) != 1 ||
		v.Vault[0].Level != 7 || !v.Vault[0].Luck {
		t.Fatalf("view = %d %+v", code, v.Vault)
	}
	if code := doJSON(t, srv, "DELETE", "/api/characters/Heroi/items/vault/0", nil, nil); code != 204 {
		t.Fatalf("delete = %d", code)
	}
	stats := Stats{Level: 20, Strength: 30, Dexterity: 30, Vitality: 30, Energy: 30, Money: 1000}
	if code := doJSON(t, srv, "PUT", "/api/characters/Heroi/stats", stats, nil); code != 204 {
		t.Fatalf("stats = %d", code)
	}

	var e map[string]string
	if code := doJSON(t, srv, "GET", "/api/characters/Ninguem", nil, &e); code != 404 || e["error"] == "" {
		t.Fatalf("missing = %d %v", code, e)
	}
	if code := doJSON(t, srv, "POST", "/api/characters/Heroi/items",
		map[string]any{"container": "vault", "index": kris.Index, "level": 99}, &e); code != 400 {
		t.Fatalf("invalid = %d %v", code, e)
	}
	if code := doJSON(t, srv, "DELETE", "/api/characters/Heroi/items/vault/x", nil, &e); code != 400 {
		t.Fatalf("bad slot = %d", code)
	}
	if err := f.st.ConnectAccount(context.Background(), f.acc.ID, "Principal", ""); err != nil {
		t.Fatal(err)
	}
	if code := doJSON(t, srv, "PUT", "/api/characters/Heroi/stats", stats, &e); code != 409 || !strings.Contains(e["error"], "online") {
		t.Fatalf("online = %d %v", code, e)
	}
}

func TestHTTPPassword(t *testing.T) {
	f := newFixture(t)
	srv := httptest.NewServer(Handler(f.ed, "segredo", slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer srv.Close()
	for _, c := range []struct {
		pw   string
		auth bool
		code int
	}{{"", false, 401}, {"errada", true, 401}, {"segredo", true, 200}} {
		req, _ := http.NewRequest("GET", srv.URL+"/api/items", nil)
		if c.auth {
			req.SetBasicAuth("admin", c.pw)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.code {
			t.Errorf("password %q: %d, want %d", c.pw, resp.StatusCode, c.code)
		}
	}
}
