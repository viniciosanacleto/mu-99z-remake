package gamedata

import (
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

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

// LoadMapManager reads Data/MapManager.txt.
func LoadMapManager(dataDir string) (map[int]MapInfo, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "MapManager.txt"))
	if err != nil {
		return nil, err
	}
	out := map[int]MapInfo{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) != 20 {
			return nil, fmt.Errorf("MapManager.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v := make([]int, 19)
		for i := range v {
			if v[i], err = l.Int(i); err != nil {
				return nil, err
			}
		}
		m := MapInfo{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8], v[9], v[10], v[11],
			v[12], v[13], v[14], v[15], v[16], v[17], v[18], l.Tokens[19]}
		out[m.Number] = m
	}
	return out, nil
}

// Gate is a Move/Gate.txt row.
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

// LoadGates reads Data/Move/Gate.txt.
func LoadGates(dataDir string) (map[int]Gate, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Move", "Gate.txt"))
	if err != nil {
		return nil, err
	}
	out := map[int]Gate{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 12 {
			return nil, fmt.Errorf("Gate.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v := make([]int, len(l.Tokens))
		for i := range v {
			if v[i], err = l.Int(i); err != nil {
				return nil, err
			}
		}
		g := Gate{Index: v[0], Flag: v[1], Map: v[2], X1: v[3], Y1: v[4], X2: v[5], Y2: v[6],
			TargetGate: v[7], TargetDir: v[8], MinLevel: v[9], MaxLevel: v[10], MinReset: v[11], MaxReset: -1, AccountLvl: -1}
		if len(v) > 12 {
			g.MaxReset = v[12]
		}
		if len(v) > 13 {
			g.AccountLvl = v[13]
		}
		out[g.Index] = g
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

// LoadMapServer reads Data/MapServerInfo.txt.
func LoadMapServer(dataDir string) (*MapServer, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "MapServerInfo.txt"))
	if err != nil {
		return nil, err
	}
	secs, err := mutext.Sections(lines)
	if err != nil {
		return nil, err
	}
	ms := &MapServer{}
	for _, s := range secs {
		for _, l := range s.Rows {
			switch s.Index {
			case 0:
				code, _ := l.Int(0)
				group, _ := l.Int(1)
				init, _ := l.Int(2)
				port, err := l.Int(4)
				if err != nil {
					return nil, err
				}
				ms.Servers = append(ms.Servers, MapServerEntry{code, group, init, l.Tokens[3], port})
			case 1:
				v := [4]int{}
				for i := range v {
					if v[i], err = l.Int(i); err != nil {
						return nil, err
					}
				}
				ms.Moves = append(ms.Moves, MapServerMove{v[0], v[1], v[2], v[3]})
			}
		}
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
