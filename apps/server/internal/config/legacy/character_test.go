package legacy

import (
	"reflect"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestDefaultClassInfo(t *testing.T) {
	cfg := convertGroup(t, "character", map[string]string{
		"Character/DefaultClassInfo.txt": "0 18 18 15 30 0 60 60 1.0 2.0 2.0 0.1 // Dark Wizard\nend\n",
	})
	c, err := gamedata.LoadClassInfo(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if dw := c[0]; dw.Energy != 30 || dw.MaxLife != 60 || dw.LevelMana != 2 || dw.EnergyToMana != float32(0.1) {
		t.Fatalf("class info = %+v", dw)
	}
	if _, err := tryConvertGroup(t, "character", map[string]string{"Character/DefaultClassInfo.txt": "0 18 18 15 30 0 x 60 1 2 2 2\nend\n"}); err == nil {
		t.Fatal("bad float accepted")
	}
}

func TestDefaultClassFreebiesStream(t *testing.T) {
	dir := convertGroup(t, "character", map[string]string{"Character/DefaultClassFreebies.txt": `0
1 500 10
end
1
0 12,015 3 255 1 1 2 63 0 60 // kept
7 12,015 0 0 0 0 0 0 0 0      // class 7 dropped
end
2
4 29 1 10 20 30 40 900
end`})
	f, err := gamedata.LoadDefaultClassFreebies(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Money[1] != 500 || f.Points[1] != 10 {
		t.Errorf("money/points %v %v", f.Money, f.Points)
	}
	want := []gamedata.FreebieItem{{Class: 0, Index: 12*512 + 15, Level: 3, Durability: 255, Skill: 1, Luck: 1, Option: 2,
		Excellent: 63, SetOption: 0, Duration: 60}}
	if !reflect.DeepEqual(f.Items, want) {
		t.Errorf("items = %+v", f.Items)
	}
	if len(f.Effects) != 1 || f.Effects[0] != (gamedata.FreebieEffect{Class: 4, Effect: 29, Type: 1, Power: [4]int{10, 20, 30, 40}, Duration: 900}) {
		t.Errorf("effects = %+v", f.Effects)
	}
}
