package gamedata

// MoveConfig is config/data/move.toml: Data/Move/*.txt.
type MoveConfig struct {
	Gate       GateFile       `comment:"Gate.txt: GateFlag 1 entrance, 2 destination; -1 = no limit; AccountLevel -1 = any"`
	Move       MoveFile       `comment:"Move.txt: the /move list; AL0..AL3 enable the destination per account level"`
	MoveSummon MoveSummonFile `comment:"MoveSummon.txt: where players can be summoned; -1 = no limit"`
}

type GateFile struct {
	Rows []GateRow `toml:",inline"`
}

type GateRow struct {
	GateIndex, GateFlag, MapNumber                 int
	RangeStartX, RangeStartY, RangeEndX, RangeEndY int
	TargetGate, TargetDir                          int
	MinLevel, MaxLevel, MinReset, MaxReset         int
	AccountLevel                                   int
	Comment                                        string `toml:",omitempty"`
}

type MoveFile struct {
	Rows []MoveRow `toml:",inline"`
}

type MoveRow struct {
	Index                                      int
	Name                                       string
	RequireMoney, MinLevel, MaxLevel, MinReset int
	MaxReset, AL0, AL1, AL2, AL3, GateNumber   int
	Comment                                    string `toml:",omitempty" legacy:"comment"`
}

type MoveSummonFile struct {
	Rows []MoveSummonConfigRow `toml:",inline"`
}

type MoveSummonConfigRow struct {
	MapNumber                                      int
	RangeStartX, RangeStartY, RangeEndX, RangeEndY int
	MinLevel, MaxLevel, MinReset, MaxReset         int
	AL0, AL1, AL2, AL3                             int
	Comment                                        string `toml:",omitempty"`
}

// LoadMoveConfig reads config/data/move.toml.
func LoadMoveConfig(configDir string) (*MoveConfig, error) {
	return loadConfig[MoveConfig](configDir, "move")
}
