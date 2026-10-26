package tomlconf

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type row struct {
	Index int    `toml:"index"`
	Name  string `toml:"name"`
}

type sample struct {
	Title string            `toml:"title" comment:"Server title"`
	Ports []int             `toml:"ports"`
	Rate  float64           `toml:"rate"`
	On    bool              `toml:"on"`
	Skip  int               `toml:"skip,omitempty"`
	Rows  []row             `toml:"rows,inline" comment:"index name"`
	Sub   sub               `toml:"sub"`
	Bags  map[string][]row  `toml:"bags"`
	Keys  map[string]string `toml:"keys"`
	Group []row             `toml:"group"`
}

type sub struct {
	Key string `toml:"Key"`
}

func sampleValue() sample {
	return sample{
		Title: `Servidor "01" \ ç`,
		Ports: []int{44405, 55900},
		Rate:  2,
		On:    true,
		Rows:  []row{{0, "Kris"}, {12, "Short Sword"}},
		Sub:   sub{Key: "v"},
		Bags:  map[string][]row{"Monster/Kundun 1": {{1, "a"}}, "Box": {{2, "b"}}},
		Keys:  map[string]string{"b": "2", "a": "1"},
		Group: []row{{5, "x"}},
	}
}

func TestMarshalLayout(t *testing.T) {
	b, err := Marshal(sampleValue())
	if err != nil {
		t.Fatal(err)
	}
	want := `# Server title
title = "Servidor \"01\" \\ ç"
ports = [44405, 55900]
rate = 2.0
on = true
# index name
rows = [
  { index = 0,  name = "Kris" },
  { index = 12, name = "Short Sword" },
]

[sub]
Key = "v"

[[bags.Box]]
index = 2
name = "b"

[[bags."Monster/Kundun 1"]]
index = 1
name = "a"

[keys]
a = "1"
b = "2"

[[group]]
index = 5
name = "x"
`
	if string(b) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b, want)
	}
}

func TestRoundTrip(t *testing.T) {
	in := sampleValue()
	in.Title = "plain ascii"
	b, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out sample
	if err := Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip:\n%+v\n%+v", in, out)
	}
}

func TestUnmarshalStrict(t *testing.T) {
	var s sample
	err := Unmarshal([]byte("title = \"x\"\ntitel = \"typo\"\n"), &s)
	if err == nil || !strings.Contains(err.Error(), "titel") || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v", err)
	}
	if err := Unmarshal([]byte("title = 3\n"), &s); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("type error = %v", err)
	}
}

func TestUnmarshalConvertsToWindows1252(t *testing.T) {
	var s struct {
		Msg  string            `toml:"msg"`
		List []string          `toml:"list"`
		Map  map[string]string `toml:"map"`
		Any  map[string]any    `toml:"any"`
	}
	src := "msg = \"Você\"\nlist = [\"ação\"]\nmap = { \"é\" = \"ñ\" }\nany = { k = \"ü\" }\n"
	if err := Unmarshal([]byte(src), &s); err != nil {
		t.Fatal(err)
	}
	if s.Msg != "Voc\xea" || s.List[0] != "a\xe7\xe3o" || s.Map["\xe9"] != "\xf1" || s.Any["k"] != "\xfc" {
		t.Fatalf("got %q %q %q %q", s.Msg, s.List, s.Map, s.Any)
	}
	if ANSI("→") != "?" {
		t.Fatalf("unsupported rune = %q", ANSI("→"))
	}
}

func TestUTF8(t *testing.T) {
	if got := UTF8("Voc\xea"); got != "Você" {
		t.Fatalf("cp1252 = %q", got)
	}
	if got := UTF8("Señor"); got != "Señor" {
		t.Fatalf("utf8 = %q", got)
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(path, []byte("title = \"t\"\nbad = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var s sample
	err := Load(path, &s)
	if err == nil || !strings.HasPrefix(err.Error(), path) {
		t.Fatalf("err = %v", err)
	}
	if err := Load(filepath.Join(t.TempDir(), "missing.toml"), &s); err == nil {
		t.Fatal("missing file: no error")
	}
}

type Head struct {
	Type int
	Name string
}

type weapon struct {
	Head
	DamageMin int
}

func TestEmbeddedStructs(t *testing.T) {
	in := struct {
		Sword []weapon `toml:",inline"`
	}{[]weapon{{Head{0, "Kris"}, 6}}}
	b, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Sword = [\n  { Type = 0, Name = \"Kris\", DamageMin = 6 },\n]\n"; string(b) != want {
		t.Fatalf("got:\n%s", b)
	}
	out := in
	out.Sword = nil
	if err := Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip %+v", out)
	}
}

func TestRowsWithOptionalLastCell(t *testing.T) {
	type r struct {
		Item    string
		Allow   int
		Comment string `toml:",omitempty"`
	}
	b, err := Marshal(struct {
		Rows []r `toml:",inline"`
	}{[]r{{"12,000", 0, "Wings of Elf"}, {"14,013", 1, ""}, {"7,1", 10, "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	want := "Rows = [\n" +
		"  { Item = \"12,000\", Allow = 0,  Comment = \"Wings of Elf\" },\n" +
		"  { Item = \"14,013\", Allow = 1 },\n" +
		"  { Item = \"7,1\",    Allow = 10, Comment = \"x\" },\n" +
		"]\n"
	if string(b) != want {
		t.Fatalf("got:\n%s", b)
	}
}

func TestSaveKeepsHeaderAndLoadUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.toml")
	os.WriteFile(path, []byte("# header 1\n# header 2\n\nTitle = \"old\"\n"), 0o644)
	type f struct{ Title string }
	if err := Save(path, f{"Você"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "# header 1\n# header 2\n\nTitle = \"Você\"\n" {
		t.Fatalf("file:\n%s", b)
	}
	var got f
	if err := LoadUTF8(path, &got); err != nil || got.Title != "Você" {
		t.Fatalf("utf8 = %q %v", got.Title, err)
	}
	if err := Load(path, &got); err != nil || got.Title != "Voc\xea" {
		t.Fatalf("ansi = %q %v", got.Title, err)
	}
	if err := LoadUTF8(filepath.Join(t.TempDir(), "none"), &got); err == nil {
		t.Fatal("missing file")
	}
}
