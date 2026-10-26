package gamedata

// LangConfig is config/data/lang.toml: Data/Lang/<language>/*.txt. The
// client language (F3 09) selects Eng, Por or Spn.
type LangConfig struct {
	Eng LangFiles
	Por LangFiles
	Spn LangFiles
}

// LangFiles are the two files of a language.
type LangFiles struct {
	Message MessageFile `comment:"Message.txt: server messages by index (printf %d / %s placeholders)"`
	Notice  NoticeFile  `comment:"Notice.txt: rotating notices; Type 0 gold / 1 blue; RepeatTime in seconds"`
}

type MessageFile struct {
	Rows []MessageRow `toml:",inline"`
}

type MessageRow struct {
	Index   int
	Text    string
	Comment string `toml:",omitempty"`
}

type NoticeFile struct {
	Rows []NoticeRow `toml:",inline"`
}

type NoticeRow struct {
	Message                                     string
	Type, Count, Opacity, Delay, ColorR, ColorG int
	ColorB, Speed, RepeatTime                   int
	Comment                                     string `toml:",omitempty" legacy:"comment"`
}

// Languages returns the language tables in client order (Eng, Por, Spn).
func (c *LangConfig) Languages() [3]*LangFiles { return [3]*LangFiles{&c.Eng, &c.Por, &c.Spn} }

// LoadLangConfig reads config/data/lang.toml.
func LoadLangConfig(configDir string) (*LangConfig, error) {
	return loadConfig[LangConfig](configDir, "lang")
}
