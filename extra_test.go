package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

// TestServe_Concurrente: 200 requests en paralelo contra el mismo archivo (race detector).
func TestServe_Concurrente(t *testing.T) {
	ts, _ := newTestServer(t, nil)
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(ts.URL + "/index.html")
			if err != nil {
				errs <- err
				return
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				errs <- &statusErr{code: resp.StatusCode}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

type statusErr struct{ code int }

func (e *statusErr) Error() string { return "status " + strconv.Itoa(e.code) }

// TestConcurrente_Stats: hits/misses consistentes bajo concurrencia (sin pérdidas).
func TestConcurrente_Stats(t *testing.T) {
	ts, s := newTestServer(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := http.Get(ts.URL + "/app.js")
			if err == nil {
				resp.Body.Close()
			}
		}(i)
	}
	wg.Wait()
	hits, misses, _, total := s.Stats().Snapshot()
	if hits+misses != total || total != 100 {
		t.Errorf("stats inconsistentes: hits=%d misses=%d total=%d (quiero 100)", hits, misses, total)
	}
}

// TestServe_TamanoGrande_Stream: un archivo mayor a la cache se sirve completo (stream).
func TestServe_TamanoGrande_Stream(t *testing.T) {
	dir := testDir(t)
	big := filepath.Join(dir, "big.bin")
	content := make([]byte, 2<<20) // 2 MB, cache de 1 MB
	for i := range content {
		content[i] = byte(i % 251)
	}
	os.WriteFile(big, content, 0o644)

	cfg := Config{Root: dir, CacheBytes: 1 << 20}
	s := New(cfg)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, body := request(t, ts.URL+"/big.bin")
	if resp.StatusCode != 200 {
		t.Fatalf("big.bin: %d", resp.StatusCode)
	}
	if len(body) != len(content) {
		t.Errorf("big.bin truncado: %d != %d", len(body), len(content))
	}
	// y no debe haberse cacheado
	if s.cache.Len() != 0 {
		t.Errorf("big.bin no debería estar en cache (%d entradas)", s.cache.Len())
	}
}

// TestServe_404NoCachea: el 404 no crea entradas en cache.
func TestServe_404NoCachea(t *testing.T) {
	ts, s := newTestServer(t, nil)
	request(t, ts.URL + "/inexistente.txt")
	request(t, ts.URL + "/index.html")
	if s.cache.Len() != 1 {
		t.Errorf("cache debería tener 1 entrada, hay %d", s.cache.Len())
	}
}
