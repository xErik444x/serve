package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Config del server.
type Config struct {
	Root       string        // directorio a servir
	Port       int           // puerto (0 = auto)
	CacheBytes int64         // tope de la cache LRU
	CORS       bool          // habilitar CORS *
	Open       bool          // 0.0.0.0 en vez de localhost
	Quiet      bool          // sin logs por request
	NoCache    bool          // deshabilitar cache (dev)
	NoBrowse   bool          // DESHABILITAR listing de dirs (default: listar como npx serve)
	LogCh      chan<- string // si no es nil, los logs de request van acá (TUI); si nil, a stdout
}

// Server es el server estático.
type Server struct {
	cfg   Config
	cache *Cache
	stats Stats
}

// New crea el server.
func New(cfg Config) *Server {
	if cfg.CacheBytes == 0 {
		cfg.CacheBytes = 64 << 20
	}
	return &Server{cfg: cfg, cache: NewCache(cfg.CacheBytes)}
}

// Stats devuelve el contador compartido (para el /__serve/stats).
func (s *Server) Stats() *Stats { return &s.stats }

// logRequest emite una línea de log. Con TUI activa (LogCh no nil) la manda
// por el canal para que Bubble Tea la renderice; si no, escribe a stdout.
// Nunca escribe directo a stdout cuando la TUI es dueña de la pantalla.
func (s *Server) logRequest(line string) {
	if s.cfg.LogCh != nil {
		select {
		case s.cfg.LogCh <- line:
		default: // TUI lenta: descartamos para no bloquear el handler
		}
		return
	}
	fmt.Printf("%s (srv)\n", line)
}

// Handler arma el http.Handler completo (público para tests).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.serveFile)
	mux.HandleFunc("/__serve/stats", s.serveStats)
	mux.HandleFunc("/__serve/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	return s.withCORS(mux)
}

// Run arranca el server y bloquea.
func (s *Server) Run() error {
	addr := s.Addr()
	return http.ListenAndServe(addr, s.Handler())
}

// Addr devuelve la dirección de escucha.
func (s *Server) Addr() string {
	host := "127.0.0.1"
	if s.cfg.Open {
		host = "0.0.0.0"
	}
	return fmt.Sprintf("%s:%d", host, s.cfg.Port)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	if !s.cfg.CORS {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serveFile es el kernel: cache -> disco, con ETag/If-None-Match y HEAD.
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
		return
	}
	s.stats.mu.Lock()
	s.stats.ReqTotal++
	s.stats.mu.Unlock()

	start := time.Now()
	urlPath := r.URL.Path
	full, ok := ResolvePath(s.cfg.Root, urlPath)
	if !ok {
		http.Error(w, "path inválido", http.StatusBadRequest)
		return
	}

	// 1) cache hit?
	var e *entry
	if !s.cfg.NoCache && ShouldCache(filepath.Ext(full)) {
		e = s.cache.Get(full)
	}
	if e == nil {
		// 2) disco: cargar (o 404 / dir listing)
		var err error
		e, err = s.load(full, urlPath, w, r)
		if err != nil {
			return // load ya escribió el error
		}
		if e == nil {
			return // dir listing ya escrito
		}
		s.stats.mu.Lock()
		s.stats.Misses++
		s.stats.mu.Unlock()
	} else {
		s.stats.mu.Lock()
		s.stats.Hits++
		s.stats.mu.Unlock()
	}

	// 3) ETag / If-None-Match
	w.Header().Set("ETag", e.etag)
	w.Header().Set("X-Serve-Go", "cache")
	if match := r.Header.Get("If-None-Match"); match != "" && etagMatch(match, e.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if !s.cfg.Quiet {
		s.logRequest(fmt.Sprintf("%s %s %d %s", r.Method, urlPath, 200, time.Since(start).Round(time.Microsecond)))
	}
	w.Header().Set("Content-Type", DetectContentType(e.path))
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", fmt.Sprint(e.size))
		return
	}
	s.stats.mu.Lock()
	s.stats.BytesSent += uint64(e.size)
	s.stats.mu.Unlock()
	w.Write(e.body)
}

// load trae del disco. Si es dir y Browse, escribe el listing y devuelve nil.
func (s *Server) load(full, urlPath string, w http.ResponseWriter, r *http.Request) (*entry, error) {
	st, err := os.Stat(full)
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return nil, err
	}
	if err != nil {
		http.Error(w, "error leyendo: "+err.Error(), http.StatusInternalServerError)
		return nil, err
	}
	if st.IsDir() {
		index := filepath.Join(full, "index.html")
		if _, err := os.Stat(index); err == nil {
			// servir el index.html del dir
			body, err := os.ReadFile(index)
			if err == nil {
				return s.cache.Put(index, body, false), nil
			}
		}
		if !s.cfg.NoBrowse {
			s.dirListing(w, full, urlPath)
			return nil, nil
		}
		http.Error(w, "directorio sin index.html (listing deshabilitado con -no-browse)", http.StatusNotFound)
		return nil, err
	}
	if st.Size() > int64(s.cfg.CacheBytes) {
		// archivo más grande que la cache: stream directo
		f, err := os.Open(full)
		if err != nil {
			http.Error(w, "error abriendo: "+err.Error(), http.StatusInternalServerError)
			return nil, err
		}
		defer f.Close()
		w.Header().Set("Content-Type", DetectContentType(full))
		w.Header().Set("Content-Length", fmt.Sprint(st.Size()))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			io.Copy(w, f)
		}
		return nil, nil // no cachear
	}
	body, err := os.ReadFile(full)
	if err != nil {
		http.Error(w, "error leyendo: "+err.Error(), http.StatusInternalServerError)
		return nil, err
	}
	return s.cache.Put(full, body, false), nil
}

// dirListing escribe un listing HTML simple.
func (s *Server) dirListing(w http.ResponseWriter, full, urlPath string) {
	entries, err := os.ReadDir(full)
	if err != nil {
		http.Error(w, "error leyendo dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=\"utf-8\"><title>" + html.EscapeString(urlPath) + "</title><style>body{font-family:system-ui;background:#0b0e14;color:#e6e9ef;max-width:720px;margin:32px auto;padding:0 16px}a{color:#7ab8ff;text-decoration:none}a:hover{text-decoration:underline}li{margin:4px 0}.d{color:#e2c04c}</style><h1>📁 " + html.EscapeString(urlPath) + "</h1><ul>")
	if urlPath != "/" {
		b.WriteString(`<li><a href="../">../</a></li>`)
	}
	for _, en := range entries {
		name := en.Name()
		if en.IsDir() {
			b.WriteString(`<li class="d"><a href="` + html.EscapeString(name) + `/">` + html.EscapeString(name) + `/</a></li>`)
		} else {
			b.WriteString(`<li><a href="` + html.EscapeString(name) + `">` + html.EscapeString(name) + `</a></li>`)
		}
	}
	b.WriteString("</ul>")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(b.String()))
}

// serveStats expone métricas (JSON) — service-to-service, sin auth (LAN unserio).
func (s *Server) serveStats(w http.ResponseWriter, r *http.Request) {
	hits, misses, bytes, total := s.stats.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hits":       hits,
		"misses":     misses,
		"bytesSent":  bytes,
		"requests":   total,
		"cacheLen":   s.cache.Len(),
		"cacheBytes": s.cache.Bytes(),
		"root":       s.cfg.Root,
	})
}

// etagMatch compara el If-None-Match con el etag (soporta * y listas).
func etagMatch(header, etag string) bool {
	if header == "*" {
		return true
	}
	for _, part := range strings.Split(header, ",") {
		if strings.TrimSpace(part) == etag {
			return true
		}
	}
	return false
}

// walkRoot cuenta archivos (para el banner del CLI).
func walkRoot(root string) (files int, totalBytes int64) {
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			files++
			if info, err := d.Info(); err == nil {
				totalBytes += info.Size()
			}
		}
		return nil
	})
	return files, totalBytes
}
