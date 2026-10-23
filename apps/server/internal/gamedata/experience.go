package gamedata

// MaxLevelTable is the size of the experience table (levels 0..1000).
const MaxLevelTable = 1000

// Experience is the level table built by FUN_0050ad00 from
// ExperienceMultiplierConstA/B (Util/ExperienceTable.txt is empty in this
// installation, so the formula is authoritative):
//
//	exp[L] = (L+9)*A*L*L                     for 1 <= L <= 1000
//	exp[L] += (k+9)*B*k*k, k = L-255         for L > 255
//
// The pet table uses (L+10)*C*L^3 for L > 2 (PetExperienceMultiplierConstA).
type Experience struct {
	Level [MaxLevelTable + 1]uint32
	Pet   [MaxLevelTable + 1]uint32
}

// NewExperience computes both tables with the original 32-bit arithmetic.
func NewExperience(a, b, petA int32) *Experience {
	e := &Experience{}
	k := int32(1)
	for l := int32(1); l <= MaxLevelTable; l++ {
		e.Level[l] = uint32((l + 9) * a * l * l)
		if l > 2 {
			e.Pet[l] = uint32((l + 10) * petA * l * l * l)
		}
		if l > 0xFF {
			e.Level[l] += uint32((k + 9) * b * k * k)
			k++
		}
	}
	return e
}

// Next returns the experience needed for the next level (obj+0xf4),
// clamped like FUN_0050be30.
func (e *Experience) Next(level int) uint32 {
	if level > MaxLevelTable {
		level = MaxLevelTable
	}
	if level < 0 {
		level = 0
	}
	return e.Level[level]
}

// Floor returns the minimum experience of a level (exp[level-1]).
func (e *Experience) Floor(level int) uint32 {
	if level <= 0 {
		return 0
	}
	if level-1 > MaxLevelTable {
		level = MaxLevelTable + 1
	}
	return e.Level[level-1]
}
