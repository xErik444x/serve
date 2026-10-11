package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
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

// Menu (compat): loop del menú sobre os.Stdin.
func Menu(title string, items []MenuItem) error {
	MenuLoop(os.Stdin, title, items)
	return nil
}

// fmtPrint escribe a stdout (helper para tests).
func fmtPrint(s string) {
	fmt.Print(s)
}

// MenuLoop es el loop del menú interactivo. r es la fuente de input
// (os.Stdin en runtime, strings.Reader en tests).
//   - opción normal: ejecuta la acción y vuelve al prompt.
//   - "s" (stats): entra en MODO STATS — el panel queda fijo en pantalla
//     y NO vuelve al menú hasta que el usuario aprieta Enter (o q para salir).
//   - "q" desde el menú (o desde stats) sale del loop.
//   - EOF del input (pipe cerrada / no TTY) sale limpio.
//
// Devuelve true si salió por q (el usuario pidió salir).
func MenuLoop(r io.Reader, title string, items []MenuItem) bool {
	sc := bufio.NewScanner(r)
	for {
		fmt.Println()
		fmt.Println(title)
		for _, it := range items {
			fmt.Printf("  [%s] %s\n", it.Key, it.Label)
		}
		fmt.Println("  [q] salir")
		fmt.Print("> ")
		if !sc.Scan() {
			return false
		}
		choice := strings.TrimSpace(strings.ToLower(sc.Text()))
		switch choice {
		case "q":
			return true
		case "":
			continue // Enter solo: re-dibuja el menú
		case "s":
			for _, it := range items {
				if strings.ToLower(it.Key) == "s" {
					if err := it.Action(); err != nil {
						fmt.Println("error:", err)
					}
					// MODO STATS: queda fijo hasta Enter/q
					fmt.Print("\n[Enter] volver al menú · [q] salir > ")
					if !sc.Scan() {
						return false
					}
					if strings.TrimSpace(strings.ToLower(sc.Text())) == "q" {
						return true
					}
				}
			}
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
