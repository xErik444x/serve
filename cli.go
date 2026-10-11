package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// FreePort pide un puerto efímero al kernel y lo suelta (0 = dejar el auto).
func FreePort(preferred int) int {
	if preferred > 0 {
		// probar el preferido primero
		if portAvailable(preferred) {
			return preferred
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return preferred
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}

func portAvailable(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// WaitForUp espera hasta que el server responda (max 3s).
func WaitForUp(port int) bool {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	deadline := time.Now().Add(3 * time.Second)
	addr := "http://127.0.0.1:" + strconv.Itoa(port) + "/__serve/health"
	for time.Now().Before(deadline) {
		if resp, err := client.Get(addr); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

// OpenBrowser abre la URL en el navegador del SO (Windows/macOS/Linux).
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// MenuItem es una opción del menú interactivo (TUI simple sin deps).
type MenuItem struct {
	Key    string // tecla a apretar
	Label  string
	Action func() error
}

// fmtPrint escribe a stdout (helper para tests).
func fmtPrint(s string) {
	fmt.Print(s)
}

// newPump lee líneas de r en una goroutine y las publica en un canal, cerrando
// el canal al llegar EOF. Deja consumir el input tanto en modo no-bloqueante
// (dashboard en vivo, vía select) como bloqueante (menú) sobre la MISMA fuente,
// sin que un bufio.Scanner de un lado se "coma" lo que el otro esperaba.
func newPump(r io.Reader) chan string {
	ch := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			ch <- sc.Text()
		}
		close(ch)
	}()
	return ch
}

// norm normaliza una línea de input (sin espacios, minúsculas).
func norm(s string) string { return strings.TrimSpace(strings.ToLower(s)) }

// LiveDashboard dibuja el panel en vivo (se refresca cada interval) y en
// paralelo escucha el input sin bloquearse. Devuelve:
//   - "quit": el usuario apretó q
//   - "menu": apretó Enter (o 'm') → abrir el menú opcional
//   - "eof" : se cerró la entrada (pipe cerrada / Ctrl-D)
func LiveDashboard(ch chan string, render func() string, interval time.Duration) string {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	drawn, prevH := false, 0
	draw := func() {
		s := render()
		if !drawn {
			fmt.Print("\033[2J\033[H") // limpiar pantalla en el primer dibujo
		} else if prevH > 0 {
			fmt.Printf("\033[%dA", prevH) // subir al tope del panel
			fmt.Print("\033[J")           // borrar de ahí hacia abajo
		}
		fmt.Print(s)
		prevH = strings.Count(s, "\n")
		drawn = true
	}
	draw()
	for {
		select {
		case <-ticker.C:
			draw()
		case raw, ok := <-ch:
			if !ok {
				return "eof"
			}
			switch norm(raw) {
			case "q":
				return "quit"
			case "", "m", "menu":
				return "menu"
			}
		}
	}
}

// MenuLoop es el menú interactivo (opcional: se abre desde el dashboard con
// Enter). Lee del MISMO canal que el dashboard.
//   - opción válida: ejecuta la acción y vuelve a mostrar el menú.
//   - Enter: vuelve al dashboard en vivo.
//   - q: sale del programa.
//   - EOF: sale del programa.
//
// Devuelve true si el usuario pidió salir (q o EOF).
func MenuLoop(ch chan string, title string, items []MenuItem) bool {
	for {
		fmt.Println()
		fmt.Println(title)
		for _, it := range items {
			fmt.Printf("  [%s] %s\n", it.Key, it.Label)
		}
		fmt.Println("  [Enter] volver a stats  ·  [q] salir")
		fmt.Print("> ")
		raw, ok := <-ch
		if !ok {
			return true
		}
		choice := norm(raw)
		switch choice {
		case "q":
			return true
		case "":
			return false // Enter → volver al dashboard
		default:
			matched := false
			for _, it := range items {
				if strings.ToLower(it.Key) == choice {
					if err := it.Action(); err != nil {
						fmt.Println("error:", err)
					}
					matched = true
					break
				}
			}
			if !matched {
				fmt.Println("opción inválida:", choice)
			}
		}
	}
}
