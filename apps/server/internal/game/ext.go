package game

// CombatStats are the derived combat values of an object (players: from
// CalcCharacter; monsters: from MonsterList.txt).
type CombatStats struct {
	DamageMinRight, DamageMaxRight int
	DamageMinLeft, DamageMaxLeft   int
	MagicDamageMin, MagicDamageMax int
	Defense                        int
	MagicDefense                   int
	AttackSuccessRate              int
	DefenseSuccessRate             int
	AttackSpeed                    int
	MagicSpeed                     int
	CriticalDamage                 int
	ExcellentDamage                int

	// combat.go (CAttack, FUN_0040ec10/FUN_0040b110) reads these as well:
	// CriticalDamage/ExcellentDamage are the rates obj+0x844/0x84c (%),
	// the bonuses are the flat additions obj+0x848/0x850.
	CriticalDamageBonus  int // obj+0x848
	ExcellentDamageBonus int // obj+0x850
	IgnoreDefenseRate    int // obj+0x83c (%)
	DoubleDamageRate     int // obj+0x834 (%)
	TripleDamageRate     int // obj+0x838 (%)
}
