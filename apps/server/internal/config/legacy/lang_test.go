package legacy

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestLangGroup(t *testing.T) {
	cfg := convertGroup(t, "lang", map[string]string{
		"Lang/Por/Message.txt": "7 \"Voc\xea n\xe3o \xe9 forte\"\n7 \"later row wins\"\nend\n",
		"Lang/Eng/Notice.txt":  "\"==GMs never ask passwords==\" 0 0 0 0 0 100 0 0 360\nend\n",
	})
	f, err := gamedata.LoadLangConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if m := f.Por.Message.Rows; len(m) != 2 || m[0].Text != "Voc\xea n\xe3o \xe9 forte" {
		t.Fatalf("Windows-1252 text must come back unchanged: %+v", m)
	}
	if n := f.Eng.Notice.Rows; len(n) != 1 || n[0].ColorG != 100 || n[0].RepeatTime != 360 {
		t.Fatalf("notices = %+v", n)
	}
	for name, body := range map[string]string{
		"Lang/Eng/Message.txt": "x \"no index\"\nend\n",
		"Lang/Spn/Notice.txt":  "\"short\" 0 0 0\nend\n",
	} {
		if _, err := tryConvertGroup(t, "lang", map[string]string{name: body}); err == nil {
			t.Errorf("%s: bad row accepted", name)
		}
	}
}
