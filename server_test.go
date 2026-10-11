package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ----- fixtures -----

func testDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":     "<h1>hola</h1>",
		"app.js":         "console.log(1)",
		"style.css":      "body{}",
		"data.json":      `{"a":1}`,
		"sub/other.html": "<p>sub</p>",
		"sub/index.html": "<p>index del sub</p>",
	}
	for name, content := range files {
		full := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func newTestServer(t *testing.T, mutate func(*Config)) (*httptest.Server, *Server) {
	t.Helper()
	cfg := Config{Root: testDir(t), CacheBytes: 1 << 20}
	if mutate != nil {
		mutate(&cfg)
	}
	s := New(cfg)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close() })
	return ts, s
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body := new(strings.Builder)
	resp.Write(body) // noop, leemos por separado
	buf := make([]byte, 0, 1<<16)
	n, _ := resp.Body.Read(buf)
	resp.Body.Close()
	return resp, string(buf[:n]) + body.String()
}

// ----- kernel: serving + content-types -----

func TestServe_IndexHTML(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	resp, body := request(t, ts.URL+"/")
	if resp.StatusCode != 200 {
		t.Fatalf("esperaba 200, hay %d", resp.StatusCode)
	}
	if !strings.Contains(body, "hola") {
		t.Errorf("index mal servido: %q", body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("content-type: %s", ct)
	}
}

func TestServe_ContentTypes(t *testing.T) {
	casos := map[string]string{
		"/app.js":     "text/javascript",
		"/style.css":  "text/css",
		"/data.json":  "application/json",
		"/inexistent": "text/plain", // 404 text
	}
	ts, _ := newTestServer(t, nil)
	for path, wantCT := range casos {
		resp, _ := request(t, ts.URL+path)
		if path == "/inexistent" {
			if resp.StatusCode != 404 {
				t.Errorf("%s: esperaba 404, hay %d", path, resp.StatusCode)
			}
			continue
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, wantCT) {
			t.Errorf("%s: content-type %s, quiero %s", path, ct, wantCT)
		}
	}
}

func TestServe_PathTraversal_Bloqueado(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	casos := []string{
		"/../etc/passwd",
		"/../../etc/passwd",
		"/..%2f..%2fetc%2fpasswd",
		"/%2e%2e/%2e%2e/etc/passwd",
	}
	for _, path := range casos {
		resp, _ := request(t, ts.URL+path)
		if resp.StatusCode == 200 {
			t.Errorf("traversal servido: %s -> %d", path, resp.StatusCode)
		}
	}
}

func TestServe_Head(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	req, _ := http.NewRequest(http.MethodHead, ts.URL+"/index.html", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("HEAD: %d", resp.StatusCode)
	}
	if cl := resp.Header.Get("Content-Length"); cl == "" {
		t.Error("HEAD sin Content-Length")
	}
}

// ----- cache -----

func TestCache_HitYMisss(t *testing.T) {
	ts, s := newTestServer(t, nil)
	request(t, ts.URL+"/index.html") // miss
	request(t, ts.URL+"/index.html") // hit
	request(t, ts.URL+"/index.html") // hit
	hits, misses, _, _ := s.Stats().Snapshot()
	if hits < 2 || misses < 1 {
		t.Errorf("hits=%d misses=%d, quiero >=2 y >=1", hits, misses)
	}
}

func TestCache_LRU_EvictPorBytes(t *testing.T) {
	// cache mínima: solo entra un archivo
	ts, s := newTestServer(t, func(c *Config) { c.CacheBytes = 16 })
	request(t, ts.URL+"/index.html")
	request(t, ts.URL+"/app.js")
	// el primero debería haber sido evictado
	if s.cache.Len() > 2 {
		t.Errorf("cache creció sin control: %d", s.cache.Len())
	}
	if s.cache.Bytes() > 16*3 { // margen
		t.Errorf("cache excedió el tope: %d", s.cache.Bytes())
	}
}

func TestCache_ETag_304(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	resp1, _ := request(t, ts.URL+"/index.html")
	etag := resp1.Header.Get("ETag")
	if etag == "" {
		t.Fatal("sin ETag")
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/index.html", nil)
	req.Header.Set("If-None-Match", etag)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotModified {
		t.Errorf("If-None-Match: esperaba 304, hay %d", resp2.StatusCode)
	}
}

// ----- dir listing / index de subdirs -----

func TestServe_SubdirIndex(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	resp, body := request(t, ts.URL+"/sub/")
	if resp.StatusCode != 200 {
		t.Fatalf("subdir: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "index del sub") {
		t.Errorf("sub/index.html no servido: %q", body)
	}
}

func TestServe_BrowseDefault_ListaDirs(t *testing.T) {
	// default: sin index.html => listing (como npx serve)
	dir := testDir(t)
	os.MkdirAll(filepath.Join(dir, "empty"), 0o755)
	os.WriteFile(filepath.Join(dir, "empty", "nota.txt"), []byte("x"), 0o644)
	cfg := Config{Root: dir, CacheBytes: 1 << 20} // NoBrowse false = default
	s := New(cfg)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	resp, body := request(t, ts.URL+"/empty/")
	if resp.StatusCode != 200 {
		t.Fatalf("browse default: esperaba 200, hay %d", resp.StatusCode)
	}
	if !strings.Contains(body, "nota.txt") {
		t.Errorf("listing no muestra nota.txt: %q", body)
	}
}

func TestServe_NoBrowse_404(t *testing.T) {
	dir := testDir(t)
	os.MkdirAll(filepath.Join(dir, "empty"), 0o755)
	cfg := Config{Root: dir, CacheBytes: 1 << 20, NoBrowse: true}
	s := New(cfg)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	resp, _ := request(t, ts.URL+"/empty/")
	if resp.StatusCode != 404 {
		t.Errorf("no-browse, dir sin index: esperaba 404, hay %d", resp.StatusCode)
	}
}

// ----- CORS -----

func TestServe_CORS(t *testing.T) {
	ts, _ := newTestServer(t, func(c *Config) { c.CORS = true })
	resp, _ := request(t, ts.URL+"/index.html")
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS header ausente")
	}
	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/index.html", nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNoContent {
		t.Errorf("CORS preflight: esperaba 204, hay %d", resp2.StatusCode)
	}
}

func TestServe_SinCORS(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	resp, _ := request(t, ts.URL+"/index.html")
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS debería estar apagado")
	}
}

// ----- health + stats -----

func TestServe_HealthYStats(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	resp, body := request(t, ts.URL+"/__serve/health")
	if resp.StatusCode != 200 || strings.TrimSpace(body) != "ok" {
		t.Errorf("health: %d %q", resp.StatusCode, body)
	}
	resp2, body2 := request(t, ts.URL+"/__serve/stats")
	if resp2.StatusCode != 200 {
		t.Fatalf("stats: %d", resp2.StatusCode)
	}
	var st map[string]interface{}
	if err := json.Unmarshal([]byte(body2), &st); err != nil {
		t.Fatalf("stats no es JSON: %v", err)
	}
	if _, ok := st["hits"]; !ok {
		t.Error("stats sin hits")
	}
}

func TestServe_MetodoPost_405(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	resp, err := http.Post(ts.URL+"/index.html", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST: esperaba 405, hay %d", resp.StatusCode)
	}
}

// ----- helpers -----

func request(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 1<<16)
	n, _ := resp.Body.Read(buf)
	return resp, strings.TrimSuffix(string(buf[:n]), "\x00")
}
