package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
)

const version = "1.4.0"

func main() {
	var (
		port    = flag.Int("p", 0, "puerto (0 = auto: 3000 o siguiente libre)")
		root    = flag.String("d", ".", "directorio a servir")
		openFl  = flag.Bool("o", false, "abrir el navegador al arrancar")
		cors    = flag.Bool("cors", false, "habilitar CORS (*), útil para APIs front")
		open    = flag.Bool("L", false, "escuchar en 0.0.0.0 (accesible desde la LAN)")
		quiet   = flag.Bool("q", false, "sin logs por request")
		nocache = flag.Bool("no-cache", false, "deshabilitar la cache (dev)")
		noBrws  = flag.Bool("no-browse", false, "DESHABILITAR listing de dirs (default: listar como npx serve)")
		noColor = flag.Bool("no-color", false, "sin colores ANSI (scripts / logs)")
		cacheMB = flag.Int64("cache", 64, "tope de la cache en MB")
		noMenu  = flag.Bool("no-menu", false, "no abrir el menú interactivo (CI / scripts)")
		ver     = flag.Bool("v", false, "versión")
	)
	flag.Usage = usage
	flag.Parse()

	colorsOn = !*noColor

	if *ver {
		fmt.Printf("serve-go %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		return
	}

	dir, err := filepath.Abs(*root)
	if err != nil {
		fatal("ruta inválida: %v", err)
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		fatal("el directorio no existe: %s", dir)
	}

	p := *port
	if p == 0 {
		p = FreePort(3000) // 3000, o el siguiente libre
	}

	// Con TUI activa el server manda sus logs por canal (nunca a stdout).
	var logCh chan string
	if !*noMenu {
		logCh = make(chan string, 256)
	}

	cfg := Config{
		Root:       dir,
		Port:       p,
		CacheBytes: *cacheMB << 20,
		CORS:       *cors,
		Open:       *open,
		Quiet:      *quiet,
		NoCache:    *nocache,
		NoBrowse:   *noBrws,
		LogCh:      logCh,
	}
	srv := New(cfg)

	url := fmt.Sprintf("http://localhost:%d", p)

	// arranque del server en goroutine
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run() }()

	if !WaitForUp(p) {
		if err := <-errCh; err != nil {
			fatal("no se pudo arrancar: %v", err)
		}
	}

	if *openFl {
		go func() { _ = OpenBrowser(url) }()
	}

	files, bytes := walkRoot(dir)

	if *noMenu {
		// Modo CI/script: cabecera plana y a esperar.
		fmt.Printf("serve-go %s  ·  %s  ·  %d archivos (%s)  ·  %s\n",
			version, dir, files, humanBytes(bytes), url)
		<-errCh
		return
	}

	items := []MenuItem{
		{"o", "abrir en el navegador", func() error { return OpenBrowser(url) }},
		{"l", "listar archivos servidos", func() error {
			filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					rel, _ := filepath.Rel(dir, path)
					fmt.Printf("  %-40s %s\n", rel, humanBytes(info.Size()))
				}
				return nil
			})
			return nil
		}},
		{"c", "limpiar la cache", func() error { srv.cache.Clear(); return nil }},
	}

	if err := runTUI(newTUIModel(srv, dir, url, items, logCh), errCh); err != nil {
		fatal("tui: %v", err)
	}
}

func usage() {
	fmt.Printf("serve-go %s — servidor estático ultrarrápido (alternativa a npx serve)\n\nUso:\n  serve [flags]\n\nFlags:\n", version)
	flag.PrintDefaults()
	fmt.Println(`
Ejemplos:
  serve                  sirve el dir actual en :3000 (o el próximo libre)
  serve -p 8080 -o       puerto 8080 y abre el navegador
  serve -d ./dist        sirve ./dist
  serve -cors            CORS para APIs front
  serve -no-browse       sin listing de dirs (404 en carpetas sin index)
  serve -L               accesible desde la LAN (0.0.0.0)
  serve -no-cache -q     sin cache ni logs (CI)`)
}

func fatal(f string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "serve-go: error: "+f+"\n", args...)
	os.Exit(1)
}

func humanBytes(n int64) string {
	const kb, mb, gb = 1 << 10, 1 << 20, 1 << 30
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func lanIP() string {
	// LAN real: la lanza el uname de las interfaces (sin hardcodear)
	ifaces, err := net.Interfaces()
	if err != nil {
		return "0.0.0.0"
	}
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "0.0.0.0"
}
