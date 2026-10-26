// Command configconv converts the configuration and data files of an
// original MuServer folder into the JSON tree under config/
// (docs/server-plan/11-arquivos-de-configuracao.md §0):
//
//	configconv -from <MuServer> -to config [-report]
//
//	1.ConnectServer/ConnectServer.ini, ServerList.dat, BlackList.txt,
//	2.DataServer/BadSyntax.txt, 3.JoinServer/JoinServer.ini -> server.json
//	<N.GameServer*>/DATA/GameServerInfo - <Name>.dat        -> gameservers/<N.GameServer*>.json
//	Data/**/*.txt|.dat|.ini                                 -> data/**/*.json
//
// Binary files (terrain, keys, SkillElect.hit, scripts) are not converted.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

func main() {
	from := flag.String("from", "", "original MuServer folder (1.ConnectServer, 4.GameServer, Data, ...)")
	to := flag.String("to", "config", "output folder")
	report := flag.Bool("report", false, "list the rows whose header comment did not name every column")
	flag.Parse()
	if *from == "" {
		fmt.Fprintln(os.Stderr, "usage: configconv -from <MuServer> -to config")
		os.Exit(2)
	}
	r, err := convert(*from, *to)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configconv:", err)
		os.Exit(1)
	}
	fmt.Printf("%d files written to %s\n", r.files, *to)
	if *report {
		for _, l := range r.unnamed {
			fmt.Println(l)
		}
	}
}

// binaryFiles are Data files that stay in their original format.
var binaryFiles = []string{"Hack/Enc*.dat", "Hack/Dec*.dat", "Common/Protect/*.dat"}

// notes are text files of the server pack that no loader reads.
var notes = []string{
	"Item/test_item.txt", "Monster/BALANCE_CHANGELOG.txt", "Shop/_ALTERACOES.txt",
	"EventItemBag/AJUSTE_JOIAS_20_PORCENTO.txt",
}

// renames gives the columns of header comments that do not split into one
// word per column.
var renames = map[string][]string{
	"ACTIVE SYNTAX PLAYER PATENTE": {"ACTIVE", "SYNTAX_PLAYER", "PATENTE"},
}

// headers names the columns of files without a header comment, after the
// fields of their Go loaders.
var headers = map[string][][]string{
	// gamedata.ItemDropRule (FUN_00492870): "cat,index" is one token.
	"Item/ItemDrop.txt": {{"ItemIndex", "Level", "Grade", "Option1", "Option2", "Option3", "Option4", "Option5",
		"Option6", "Duration", "Map", "Monster", "MinLevel", "MaxLevel",
		"DropRate_AL0", "DropRate_AL1", "DropRate_AL2", "DropRate_AL3"}},
	// Section 0: default price per item category.
	"Custom/CustomXShop.txt": {{"Type", "Preco"}},
}

type result struct {
	files   int
	unnamed []string
}

func convert(from, to string) (*result, error) {
	r := &result{}
	if err := convertServer(from, to, r); err != nil {
		return nil, err
	}
	instances, err := filepath.Glob(filepath.Join(from, "*", "DATA", "GameServerInfo - Common.dat"))
	if err != nil {
		return nil, err
	}
	for _, p := range instances {
		dir := filepath.Dir(p)
		name := filepath.Base(filepath.Dir(dir))
		if err := convertInstance(dir, filepath.Join(to, "gameservers", name+".json")); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		r.files++
	}
	if err := convertData(filepath.Join(from, "Data"), filepath.Join(to, "data"), r); err != nil {
		return nil, err
	}
	return r, nil
}

func convertServer(from, to string, r *result) error {
	f := &jsonconf.ServerFile{}
	var err error
	if f.ConnectServer, err = settingsFile(filepath.Join(from, "1.ConnectServer", "ConnectServer.ini")); err != nil {
		return err
	}
	if f.JoinServer, err = settingsFile(filepath.Join(from, "3.JoinServer", "JoinServer.ini")); err != nil {
		return err
	}
	for _, t := range []struct {
		dst  **jsonconf.Table
		path string
	}{
		{&f.ServerList, filepath.Join(from, "1.ConnectServer", "ServerList.dat")},
		{&f.BlackList, filepath.Join(from, "1.ConnectServer", "BlackList.txt")},
		{&f.BadSyntax, filepath.Join(from, "2.DataServer", "BadSyntax.txt")},
	} {
		if *t.dst, err = tableFile(t.path, mutext.Tokenize, nil, r); err != nil {
			return err
		}
	}
	r.files++
	return jsonconf.WriteServer(filepath.Join(to, "server.json"), f)
}

// convertInstance joins the eight GameServerInfo files of an instance into
// one settings file whose sections are the file names.
func convertInstance(dir, dst string) error {
	out := &jsonconf.Settings{}
	for _, name := range gsconfig.Files {
		s, err := settingsFile(filepath.Join(dir, "GameServerInfo - "+name+".dat"))
		if err != nil {
			return err
		}
		sec := jsonconf.SettingsSection{Name: name}
		for _, src := range s.Sections {
			if !strings.EqualFold(src.Name, "GameServerInfo") {
				return fmt.Errorf("GameServerInfo - %s.dat: unexpected section [%s]", name, src.Name)
			}
			sec.Keys = append(sec.Keys, src.Keys...)
		}
		out.Sections = append(out.Sections, sec)
	}
	return jsonconf.WriteSettings(dst, out)
}

func convertData(from, to string, r *result) error {
	var paths []string
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".txt", ".dat", ".ini":
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, p := range paths {
		rel, _ := filepath.Rel(from, p)
		rel = filepath.ToSlash(rel)
		if matchAny(binaryFiles, rel) || matchAny(notes, rel) {
			continue
		}
		dst := filepath.Join(to, filepath.FromSlash(strings.TrimSuffix(rel, filepath.Ext(rel))+".json"))
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if jsonconf.IsINI(data) {
			s, err := jsonconf.SettingsFromINI(data)
			if err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			if err := jsonconf.WriteSettings(dst, s); err != nil {
				return err
			}
		} else {
			t, err := tableFile(p, jsonconf.TokenizerFor(rel), headers[rel], r)
			if err != nil {
				return err
			}
			if err := jsonconf.WriteTable(dst, t); err != nil {
				return err
			}
		}
		r.files++
	}
	return nil
}

func settingsFile(path string) (*jsonconf.Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s, err := jsonconf.SettingsFromINI(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

func tableFile(path string, tok jsonconf.Tokenizer, known [][]string, r *result) (*jsonconf.Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t, err := jsonconf.TableFromText(data, tok, jsonconf.Hints{Rename: renames, Headers: known})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// Report the first row that shows each unnamed column.
	seen := map[string]bool{}
	for _, s := range t.Sections {
		for _, row := range s.Rows {
			report := false
			for _, c := range row {
				if unnamedColumn.MatchString(c.Name) && !seen[c.Name] {
					seen[c.Name], report = true, true
				}
			}
			if report {
				r.unnamed = append(r.unnamed, fmt.Sprintf("%s: %s", path, rowNames(row)))
			}
		}
	}
	return t, nil
}

var unnamedColumn = regexp.MustCompile(`^Col[0-9]+$`)

func rowNames(row jsonconf.Row) string {
	var b strings.Builder
	for i, c := range row {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(c.Name + "=" + c.Value)
	}
	return b.String()
}

func matchAny(patterns []string, rel string) bool {
	for _, p := range patterns {
		if ok, err := filepath.Match(p, rel); err == nil && ok {
			return true
		}
	}
	return false
}
