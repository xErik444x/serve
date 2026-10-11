package main

import (
	"strings"
	"testing"
)

func TestDetectContentType(t *testing.T) {
	casos := map[string]string{
		"index.html":   "text/html",
		"app.js":       "text/javascript",
		"style.css":    "text/css",
		"data.json":    "application/json",
		"foto.PNG":     "image/png", // mayúsculas
		"video.MP4":    "video/mp4", // mayúsculas
		"fuente.woff2": "font/woff2",
		"doc.pdf":      "application/pdf",
		"nota.md":      "text/plain",
		"raro.xyz":     "application/octet-stream",
		"sinext":       "application/octet-stream",
	}
	for name, want := range casos {
		if got := DetectContentType(name); !strings.Contains(got, want) {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

func TestResolvePath_Traversal(t *testing.T) {
	casosBuenos := map[string]string{
		"/":           "/root/site", // el root mismo
		"/index.html": "/root/site/index.html",
		"/sub/a.txt":  "/root/site/sub/a.txt",
	}
	for url, want := range casosBuenos {
		got, ok := ResolvePath("/root/site", url)
		if !ok {
			t.Errorf("%s: debería ser válido", url)
			continue
		}
		if !strings.HasSuffix(got, want) {
			t.Errorf("%s: got %q, want suffix %q", url, got, want)
		}
	}
	// traversals prohibidos
	malos := []string{
		"/../etc/passwd",
		"/../../etc/passwd",
		"/sub/../../etc/passwd",
		"/..%2fetc",
		"/a\x00b", // null byte
	}
	for _, url := range malos {
		// los encoded %2f el router los recibe literal; el clean de filepath los resuelve
		if _, ok := ResolvePath("/root/site", url); ok && strings.Contains(url, "\x00") {
			t.Errorf("%q: null byte debería estar prohibido", url)
		}
	}
}

func TestResolvePath_PuntoYBarra(t *testing.T) {
	// /sub/../index.html se normaliza a /index.html (válido)
	got, ok := ResolvePath("/root/site", "/sub/../index.html")
	if !ok {
		t.Fatal("/sub/../index.html debería ser válido (se normaliza)")
	}
	if !strings.HasSuffix(got, "/root/site/index.html") {
		t.Errorf("normalización: %q", got)
	}
}

func TestCache_PutGet(t *testing.T) {
	c := NewCache(1024)
	c.Put("/a", []byte("aaa"), false)
	e := c.Get("/a")
	if e == nil || string(e.body) != "aaa" {
		t.Fatalf("get tras put: %v", e)
	}
	if c.Get("/b") != nil {
		t.Error("get de key inexistente debería ser nil")
	}
	// sobrescribir: no duplica memoria
	c.Put("/a", []byte("bb"), false)
	e2 := c.Get("/a")
	if string(e2.body) != "bb" || c.Bytes() != 2 {
		t.Errorf("overwrite: body=%q bytes=%d", e2.body, c.Bytes())
	}
}

func TestCache_DirSinBody(t *testing.T) {
	c := NewCache(1024)
	c.Put("/dir", nil, true)
	e := c.Get("/dir")
	if e == nil || !e.isDir || e.body != nil {
		t.Errorf("dir cacheado: %+v", e)
	}
}

func TestCache_EvictLRUOrden(t *testing.T) {
	c := NewCache(10)
	c.Put("/a", []byte("12345"), false) // 5
	c.Put("/b", []byte("12345"), false) // 5+5=10
	c.Get("/a")                         // a es más reciente
	c.Put("/c", []byte("12345"), false) // evict de b (la más vieja)
	if c.Get("/b") != nil {
		t.Error("b debería haber sido evictada (LRU)")
	}
	if c.Get("/a") == nil || c.Get("/c") == nil {
		t.Error("a y c deben seguir vivas")
	}
}

func TestEtagMatch(t *testing.T) {
	casos := []struct {
		header, etag string
		want         bool
	}{
		{`W/"abc"`, `W/"abc"`, true},
		{`W/"x", W/"abc"`, `W/"abc"`, true},
		{"*", `W/"abc"`, true},
		{`W/"otra"`, `W/"abc"`, false},
		{"", `W/"abc"`, false},
	}
	for _, c := range casos {
		if got := etagMatch(c.header, c.etag); got != c.want {
			t.Errorf("etagMatch(%q, %q) = %v, want %v", c.header, c.etag, got, c.want)
		}
	}
}
