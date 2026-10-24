package editor

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
)

//go:embed web
var webFS embed.FS

// Handler serves the editor page and its JSON API:
//
//	GET    /api/items                           item catalog
//	GET    /api/characters?q=                   search by name or login
//	GET    /api/characters/{name}               character, inventory, vault
//	PUT    /api/characters/{name}/stats         Stats
//	POST   /api/characters/{name}/items         NewItem
//	DELETE /api/characters/{name}/items/{container}/{slot}
//
// A non-empty password enables HTTP basic auth (any user name).
func Handler(e *Editor, password string, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(static))

	type catalogResponse struct {
		Categories [16]string    `json:"categories"`
		Items      []CatalogItem `json:"items"`
	}
	mux.HandleFunc("GET /api/items", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, catalogResponse{CategoryNames, e.Catalog().Items})
	})
	mux.HandleFunc("GET /api/characters", func(w http.ResponseWriter, r *http.Request) {
		refs, err := e.Search(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			writeError(w, log, err)
			return
		}
		type row struct {
			Name   string `json:"name"`
			Login  string `json:"login"`
			Level  int    `json:"level"`
			Class  byte   `json:"class"`
			Resets int    `json:"resets"`
			Online bool   `json:"online"`
		}
		out := []row{}
		for _, c := range refs {
			out = append(out, row{c.Name, c.Login, c.Level, c.Class, c.Resets, c.Online})
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("GET /api/characters/{name}", func(w http.ResponseWriter, r *http.Request) {
		v, err := e.Character(r.Context(), r.PathValue("name"))
		if err != nil {
			writeError(w, log, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("PUT /api/characters/{name}/stats", func(w http.ResponseWriter, r *http.Request) {
		var s Stats
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeError(w, log, ErrInvalid)
			return
		}
		if err := e.UpdateStats(r.Context(), r.PathValue("name"), s); err != nil {
			writeError(w, log, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/characters/{name}/items", func(w http.ResponseWriter, r *http.Request) {
		n := NewItem{Slot: -1}
		if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
			writeError(w, log, ErrInvalid)
			return
		}
		slot, err := e.AddItem(r.Context(), r.PathValue("name"), n)
		if err != nil {
			writeError(w, log, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]int{"slot": slot})
	})
	mux.HandleFunc("DELETE /api/characters/{name}/items/{container}/{slot}", func(w http.ResponseWriter, r *http.Request) {
		slot, err := strconv.Atoi(r.PathValue("slot"))
		if err != nil {
			writeError(w, log, ErrInvalid)
			return
		}
		if err := e.RemoveItem(r.Context(), r.PathValue("name"), Container(r.PathValue("container")), slot); err != nil {
			writeError(w, log, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if password == "" {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pw, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(pw), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="MU Editor"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, log *slog.Logger, err error) {
	status := http.StatusInternalServerError
	msg := "erro interno"
	switch {
	case errors.Is(err, ErrNotFound):
		status, msg = http.StatusNotFound, err.Error()
	case errors.Is(err, ErrOnline):
		status, msg = http.StatusConflict, err.Error()
	case errors.Is(err, ErrNoSpace):
		status, msg = http.StatusConflict, err.Error()
	case errors.Is(err, ErrInvalid):
		status, msg = http.StatusBadRequest, err.Error()
	default:
		log.Error("editor", "err", err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}
