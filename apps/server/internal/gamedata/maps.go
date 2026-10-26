package gamedata

import ()

// MapInfo is a MapManager.txt row. Columns written "*" read as -1.
type MapInfo struct {
	Number          int
	NonPK           int
	ViewRange       int
	ExperienceRate  int
	ItemDropRate    int
	MoneyDropRate   int
	ExcItemDropRate int
	SetItemDropRate int
	ItemDrop        int
	PKItemDrop      int
	MonsterHPBar    int
	TradeEnable     int
	PShopEnable     int
	ChatEnable      int
	PartyEnable     int
	DuelEnable      int
	GuildWarEnable  int
	SpawnGate       int
	RespawnGate     int
	Name            string
}

// DefaultViewRange is used when a map has no MapManager row (local_14 = 0xc).
const DefaultViewRange = 12

// LoadMapManager reads the MapManager table of config/data/common.toml.
func LoadMapManager(configDir string) (map[int]MapInfo, error) {
	f, err := LoadCommonConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]MapInfo{}
	for _, r := range f.MapManager.Rows {
		out[r.MapNumber] = MapInfo{r.MapNumber, r.NonPK, r.ViewRange, r.ExperienceRate, r.ItemDropRate, r.MoneyDropRate,
			r.ExcItemDropRate, r.SetItemDropRate, r.ItemDrop, r.PKItemDrop, r.MonsterHPBar, r.TradeEnable, r.PShopEnable,
			r.ChatEnable, r.PartyEnable, r.DuelEnable, r.GuildWarEnable, r.SpawnGate, r.RespawnGate, r.Name}
	}
	return out, nil
}

// Gate is a Gate.txt row.
type Gate struct {
	Index      int
	Flag       int // 1 = entrance, 2 = destination (0 = plain destination)
	Map        int
	X1, Y1     int
	X2, Y2     int
	TargetGate int
	TargetDir  int
	MinLevel   int
	MaxLevel   int
	MinReset   int
	MaxReset   int
	AccountLvl int
}

// LoadGates reads the Gate table of config/data/move.toml.
func LoadGates(configDir string) (map[int]Gate, error) {
	f, err := LoadMoveConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]Gate{}
	for _, r := range f.Gate.Rows {
		out[r.GateIndex] = Gate{Index: r.GateIndex, Flag: r.GateFlag, Map: r.MapNumber, X1: r.RangeStartX,
			Y1: r.RangeStartY, X2: r.RangeEndX, Y2: r.RangeEndY, TargetGate: r.TargetGate, TargetDir: r.TargetDir,
			MinLevel: r.MinLevel, MaxLevel: r.MaxLevel, MinReset: r.MinReset, MaxReset: r.MaxReset, AccountLvl: r.AccountLevel}
	}
	return out, nil
}

// MapServer is MapServerInfo.txt: server groups (section 0) and per-map
// redirections (section 1).
type MapServer struct {
	Servers []MapServerEntry
	Moves   []MapServerMove
}

type MapServerEntry struct {
	Code, Group, InitSetVal int
	Address                 string
	Port                    int
}

type MapServerMove struct {
	ServerCode int
	MoveOption int // 0 = redirect to NextServerCode, 1 = stay
	Map        int
	NextServer int // -1 = "*"
}

// LoadMapServer reads the MapServerInfo table of config/data/common.toml.
func LoadMapServer(configDir string) (*MapServer, error) {
	f, err := LoadCommonConfig(configDir)
	if err != nil {
		return nil, err
	}
	ms := &MapServer{}
	for _, r := range f.MapServerInfo.Server {
		ms.Servers = append(ms.Servers, MapServerEntry{r.ServerCode, r.ServerGroup, r.InitSetVal, r.IpAddress, r.Port})
	}
	for _, r := range f.MapServerInfo.Move {
		ms.Moves = append(ms.Moves, MapServerMove{r.ServerCode, r.MoveOption, r.NextMap, r.NextServerCode})
	}
	return ms, nil
}

// ServerForMap returns the server code that hosts mp when seen from
// serverCode, or serverCode itself when there is no redirection.
func (m *MapServer) ServerForMap(serverCode, mp int) int {
	for _, mv := range m.Moves {
		if mv.ServerCode == serverCode && mv.Map == mp && mv.MoveOption == 0 && mv.NextServer >= 0 {
			return mv.NextServer
		}
	}
	return serverCode
}
