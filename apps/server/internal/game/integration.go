package game

// Cross-module wiring between modules written separately.

func registerIntegration(h *hooks) {
	// Monster skills (monster_skill.go) apply and remove effects through
	// the effect system (effect.go, FUN_00478ae0 / FUN_00478d30).
	h.addEffect = func(w *World, o *Object, effect, value, seconds int) bool {
		return w.addEffect(o, effect, seconds, [4]int{value}, true)
	}
	h.removeEffect = func(w *World, o *Object, effect int) { w.removeEffect(o, effect) }
}
