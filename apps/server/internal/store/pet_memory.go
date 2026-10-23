package store

import "context"

func (m *Memory) petTable() map[uint32]PetInfo {
	if m.pets == nil {
		m.pets = map[uint32]PetInfo{}
	}
	return m.pets
}

func (m *Memory) LoadPetInfo(_ context.Context, serials []uint32) ([]PetInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.petTable()
	out := make([]PetInfo, 0, len(serials))
	for _, s := range serials {
		p, ok := t[s]
		if !ok {
			p = DefaultPetInfo(s)
			t[s] = p
		}
		out = append(out, p)
	}
	return out, nil
}

func (m *Memory) SavePetInfo(_ context.Context, pets []PetInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.petTable()
	for _, p := range pets {
		t[p.Serial] = p
	}
	return nil
}

func (m *Memory) CreatePetInfo(_ context.Context, p PetInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.petTable()
	if _, ok := t[p.Serial]; !ok {
		t[p.Serial] = p
	}
	return nil
}
