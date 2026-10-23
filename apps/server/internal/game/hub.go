package game

import (
	"strings"
	"sync"
)

// Hub connects the game instances of the process. It replaces the
// DataServer's online-character map (DataServer 0x70 enter / 0x71 leave,
// FUN_0040fdb0 / FUN_0040ff30) and the DataServer relays between game
// servers (friend notifications, cross-server whisper 0x72).
type Hub struct {
	mu     sync.Mutex
	worlds map[int]*World
	online map[string]OnlineChar // lower(name)
}

// OnlineChar is an entry of the online map.
type OnlineChar struct {
	Name       string
	Account    string
	ServerCode int
	Index      int
}

func NewHub() *Hub {
	return &Hub{worlds: map[int]*World{}, online: map[string]OnlineChar{}}
}

func (h *Hub) register(w *World) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.worlds[w.cfg.ServerCode] = w
}

// Enter is DataServer 0x70: the first registration of a name wins.
func (h *Hub) Enter(c OnlineChar) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := strings.ToLower(c.Name)
	if _, ok := h.online[key]; ok {
		return false
	}
	h.online[key] = c
	return true
}

// Leave is DataServer 0x71: only the matching index/server removes it.
func (h *Hub) Leave(name string, serverCode, index int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := strings.ToLower(name)
	c, ok := h.online[key]
	if !ok || c.ServerCode != serverCode || c.Index != index {
		return false
	}
	delete(h.online, key)
	return true
}

// Lookup is FUN_00407550 on the online map.
func (h *Hub) Lookup(name string) (OnlineChar, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.online[strings.ToLower(name)]
	return c, ok
}

// ServerOf is FUN_0041c770: the server code of an online name, or 0xFF.
func (h *Hub) ServerOf(name string) byte {
	if c, ok := h.Lookup(name); ok {
		return byte(c.ServerCode)
	}
	return 0xFF
}

// Post runs f on the world goroutine of a server code (FUN_0041c810 finds
// the game server connection, FUN_0041bab0 sends to it).
func (h *Hub) Post(serverCode int, f func(*World)) bool {
	h.mu.Lock()
	w := h.worlds[serverCode]
	h.mu.Unlock()
	if w == nil {
		return false
	}
	w.post(func() { f(w) })
	return true
}

// PostToChar runs f on the world holding an online character, with its
// object if it is still the same session.
func (h *Hub) PostToChar(name string, f func(*World, *Object)) bool {
	c, ok := h.Lookup(name)
	if !ok {
		return false
	}
	return h.Post(c.ServerCode, func(w *World) {
		o := w.objects[c.Index]
		if o.State == StatePlaying && o.P != nil && o.P.Char != nil && strings.EqualFold(o.P.Char.Name, c.Name) {
			f(w, o)
		}
	})
}

// whisperRelay is the DataServer 0x72 / 0x73 relay: a whisper to a name
// that is not on this instance goes to the instance holding it (exact name
// match, like the local lookup); false makes the sender get C1 04 0C.
func whisperRelay(w *World, from *Object, target string, msg []byte) bool {
	if w.hub == nil {
		return false
	}
	c, ok := w.hub.Lookup(target)
	if !ok || c.Name != target || c.ServerCode == w.cfg.ServerCode {
		return false
	}
	name, text := from.P.Char.Name, append([]byte(nil), msg...)
	return w.hub.PostToChar(target, func(tw *World, t *Object) { tw.deliverWhisper(t, name, text) })
}

func init() { globalWhisperHook = whisperRelay }
