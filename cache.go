package main

import (
	"container/list"
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// entry es una entrada de la cache LRU: contenido en memoria + ETag.
type entry struct {
	path  string
	body  []byte
	etag  string
	size  int
	isDir bool
}

// Cache es un caché LRU de archivos estáticos, seguro para concurrencia.
// Capacidad limitada por bytes totales (no por cantidad) para acotar memoria.
type Cache struct {
	mu       sync.Mutex
	maxBytes int64
	curBytes int64
	items    map[string]*list.Element
	order    *list.List // front = más reciente
}

// NewCache crea una cache con tope de memoria. maxBytes <= 0 => 64 MB.
func NewCache(maxBytes int64) *Cache {
	if maxBytes <= 0 {
		maxBytes = 64 << 20
	}
	return &Cache{
		maxBytes: maxBytes,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Get devuelve el contenido cacheado o nil si no está.
func (c *Cache) Get(path string) *entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[path]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*entry)
	}
	return nil
}

// Put carga un archivo a la cache. Los directorios se cachean sin cuerpo (solo meta).
func (c *Cache) Put(path string, body []byte, isDir bool) *entry {
	e := &entry{
		path:  path,
		body:  body,
		size:  len(body),
		isDir: isDir,
		etag:  etagOf(body),
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[path]; ok {
		c.curBytes -= int64(el.Value.(*entry).size)
		el.Value = e
		c.order.MoveToFront(el)
	} else {
		c.items[path] = c.order.PushFront(e)
	}
	c.curBytes += int64(e.size)
	c.evict()
	return e
}

// evict saca los más viejos hasta quedar bajo el tope. LLAMAR CON el lock tomado.
func (c *Cache) evict() {
	for c.curBytes > c.maxBytes && c.order.Len() > 0 {
		el := c.order.Back()
		if el == nil {
			return
		}
		e := el.Value.(*entry)
		c.curBytes -= int64(e.size)
		c.order.Remove(el)
		delete(c.items, e.path)
	}
}

// Len devuelve la cantidad de entradas cacheadas (para tests y stats).
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Bytes devuelve la memoria usada por la cache.
func (c *Cache) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.curBytes
}

// Clear vacía la cache por completo (menú interactivo / -no-cache).
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.order = list.New()
	c.curBytes = 0
}

// Stats son las métricas del server (acumula hits/misses/bytes).
type Stats struct {
	mu        sync.Mutex
	Hits      uint64
	Misses    uint64
	BytesSent uint64
	ReqTotal  uint64
}

// Snapshot copia los contadores atómicamente.
func (s *Stats) Snapshot() (hits, misses, bytes, total uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Hits, s.Misses, s.BytesSent, s.ReqTotal
}

// etagOf genera un ETag débil y corto (sha1 truncado, suficiente para estáticos).
func etagOf(b []byte) string {
	if len(b) == 0 {
		return `W/"empty"`
	}
	sum := sha1.Sum(b)
	return fmt.Sprintf(`W/"%x"`, sum[:8])
}

// ResolvePath une root + URL path de forma segura (anti path traversal).
// Devuelve el path limpio y si es seguro servirlo.
func ResolvePath(root, urlPath string) (string, bool) {
	if strings.Contains(urlPath, "\x00") {
		return "", false
	}
	clean := filepath.Clean("/" + strings.TrimPrefix(urlPath, "/"))
	full := filepath.Join(root, clean)
	// defensa extra: el resolved debe quedar dentro del root
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(rootAbs, fullAbs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return fullAbs, true
}

// ShouldCache decide si un mdimetype se puede cachear (nada executables raros).
func ShouldCache(ext string) bool {
	switch strings.ToLower(ext) {
	case ".go", ".env", ".key", ".pem", ".sqlite", ".db":
		return false
	}
	return true
}

// DetectContentType por extensión (rápido, sin syscall por request).
func DetectContentType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".pdf":
		return "application/pdf"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	case ".txt", ".md", ".log":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// Exists chequea existencia sin abrir.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
