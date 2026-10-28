package guild

// E6 / E1 result codes seen in the original.
const (
	RelationCastle      = 0x10 // Castle Siege running (msg 742)
	RelationNotMaster   = 0x11 // both sides must be guild masters
	RelationRivalUnion  = 0x15 // hostility asked to an allied guild
	RelationRivalOfMine = 0x16 // union asked by a rival guild
	RelationInUnion     = 0x17 // already in a union (or castle owner)
	RelationHasRival    = 0x18 // a rival already exists
	RelationNoRelation  = 0x19 // nothing to break off
	RelationNotUnionMas = 0x1B // the target is a union member, not its master
	RelationNotRivals   = 0x1C // the rivals are not each other's
	RelationFewMembers  = 0x1D // fewer members than GuildAllianceMinUser
	RelationUnionFull   = 0x1E // GuildAllianceMaxCount reached
	RelationRefused     = 0x20 // the other master said no
	RelationSelf        = 0x21 // the union master kicking itself
)
