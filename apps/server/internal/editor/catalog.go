// Package editor is the web character editor (cmd/mueditor), the
// replacement for the SQL Server MuEdit tool: it lists characters, edits
// stats and zen, and adds or removes inventory and vault items directly in
// the store, using the same item encoding and Item.txt / ItemOption.txt data
// as the GameServer.
package editor

import (
	"fmt"
	"sort"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// CatalogItem is one Item.txt entry with the options ItemOption.txt allows.
type CatalogItem struct {
	Index    int         `json:"index"`
	Category int         `json:"cat"`
	Type     int         `json:"type"`
	Name     string      `json:"name"`
	Width    int         `json:"w"`
	Height   int         `json:"h"`
	Slot     int         `json:"slot"` // Item.txt Slot, -1 = not equippable
	Skill    bool        `json:"skill"`
	Luck     bool        `json:"luck"`
	Option   string      `json:"option,omitempty"` // additional option label, "" = none
	Exc      []ExcOption `json:"exc,omitempty"`
	Stack    bool        `json:"stack"` // durability is a quantity (category 14)
}

// ExcOption is one excellent bit an item accepts.
type ExcOption struct {
	Bit   int    `json:"bit"` // mask 1..32
	Label string `json:"label"`
}

// Catalog is the item table of the editor.
type Catalog struct {
	Items []CatalogItem
	byIdx map[int]CatalogItem
	info  map[int]gamedata.ItemInfo
}

// CategoryNames labels the 16 Item.txt sections.
var CategoryNames = [16]string{
	"Espadas", "Machados", "Maças e cetros", "Lanças", "Arcos e bestas", "Cajados",
	"Escudos", "Elmos", "Armaduras", "Calças", "Luvas", "Botas",
	"Asas e orbes", "Acessórios e pets", "Poções e joias", "Pergaminhos",
}

// NewCatalog builds the catalog from Item.txt and ItemOption.txt rows.
func NewCatalog(items map[int]gamedata.ItemInfo, options map[int][]gamedata.ItemOptionRule) *Catalog {
	c := &Catalog{byIdx: map[int]CatalogItem{}, info: items}
	for idx, info := range items {
		ci := CatalogItem{
			Index: idx, Category: info.Category, Type: info.Type, Name: info.Name,
			Width: info.Width, Height: info.Height, Slot: info.Slot,
			Stack: info.Category == 14,
		}
		ci.Skill = info.Skill > 0 && optionRule(options[0], idx, 0) != nil
		ci.Luck = optionRule(options[1], idx, 0) != nil
		if r := optionRule(options[2], idx, 0); r != nil {
			ci.Option = optionLabel(r.OptionIndex, r.Value, true)
		}
		for bit := 1; bit <= 32; bit <<= 1 {
			if r := excRule(options, idx, bit); r != nil {
				label := optionLabel(r.OptionIndex, r.Value, r.Index == 2)
				if r.Index == 2 {
					label = "Opção adicional vira: " + label
				}
				ci.Exc = append(ci.Exc, ExcOption{Bit: bit, Label: label})
			}
		}
		c.Items = append(c.Items, ci)
		c.byIdx[idx] = ci
	}
	sort.Slice(c.Items, func(i, j int) bool { return c.Items[i].Index < c.Items[j].Index })
	return c
}

// Item returns the catalog entry of an index.
func (c *Catalog) Item(index int) (CatalogItem, bool) {
	ci, ok := c.byIdx[index]
	return ci, ok
}

// optionRule is the last rule of a slot (FUN_0049d390: the last match wins)
// that applies to the item with skill, luck and option 1 and the given
// excellent mask, among the rules that do not depend on an excellent bit.
func optionRule(rules []gamedata.ItemOptionRule, index int, exc byte) *gamedata.ItemOptionRule {
	var found *gamedata.ItemOptionRule
	for i := range rules {
		r := rules[i]
		if r.Excellent == -1 && r.Matches(index, 1, 1, 1, exc) {
			found = &rules[i]
		}
	}
	return found
}

// excRule finds the rule enabled by one excellent bit, in any option slot.
func excRule(options map[int][]gamedata.ItemOptionRule, index, bit int) *gamedata.ItemOptionRule {
	var found *gamedata.ItemOptionRule
	for slot := 0; slot <= 13; slot++ {
		rules := options[slot]
		for i := range rules {
			r := rules[i]
			if r.Excellent != -1 && r.Excellent&bit != 0 && r.Matches(index, 1, 1, 1, byte(bit)) {
				found = &rules[i]
			}
		}
	}
	return found
}

// optionLabel describes an ItemOption effect code (game/stats.go
// applyOption). additional marks the per-level option, whose value is
// multiplied by the option level (+1..+7).
func optionLabel(code, v int, additional bool) string {
	per := ""
	if additional {
		per = " por nível da opção"
	}
	switch code {
	case 80:
		return fmt.Sprintf("Dano +%d%s", v, per)
	case 81:
		return fmt.Sprintf("Dano mágico +%d%s", v, per)
	case 82:
		return fmt.Sprintf("Taxa de defesa +%d%s", v, per)
	case 83:
		return fmt.Sprintf("Defesa +%d%s", v, per)
	case 84:
		return fmt.Sprintf("Dano crítico +%d%%", v)
	case 85:
		return fmt.Sprintf("Recuperação de HP +%d%%%s", v, per)
	case 86:
		return fmt.Sprintf("Zen dropado +%d%%", v)
	case 87:
		return fmt.Sprintf("Taxa de defesa +%d%%", v)
	case 88:
		return fmt.Sprintf("Reflete %d%% do dano", v)
	case 89:
		return fmt.Sprintf("Reduz %d%% do dano", v)
	case 90:
		return fmt.Sprintf("Mana máxima +%d%%", v)
	case 91:
		return fmt.Sprintf("HP máximo +%d%%", v)
	case 92:
		return fmt.Sprintf("Chance de dano excelente +%d%%", v)
	case 93:
		return fmt.Sprintf("Dano +nível/%d", v)
	case 94:
		return fmt.Sprintf("Dano +%d%%", v)
	case 95:
		return fmt.Sprintf("Dano mágico +nível/%d", v)
	case 96:
		return fmt.Sprintf("Dano mágico +%d%%", v)
	case 97:
		return fmt.Sprintf("Velocidade de ataque +%d", v)
	case 98:
		return fmt.Sprintf("HP ao matar monstro (%d)", v)
	case 99:
		return fmt.Sprintf("Mana ao matar monstro (%d)", v)
	case 100:
		return fmt.Sprintf("HP +50 +%d por nível do item", v)
	case 101:
		return fmt.Sprintf("Mana +50 +%d por nível do item", v)
	case 102:
		return fmt.Sprintf("Ignora %d%% da defesa", v)
	case 103:
		return fmt.Sprintf("AG máximo +%d", v)
	case 104:
		return fmt.Sprintf("AG máximo +%d%%", v)
	case 105:
		return fmt.Sprintf("Comando +10 +%d por nível do item", v)
	}
	return fmt.Sprintf("Opção %d (%d)", code, v)
}
