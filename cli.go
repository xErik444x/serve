package main

import (
	"fmt"
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

// Estructura de menú interactivo (TUI simple sin deps).
type MenuItem struct {
	Key    string // tecla a apretar
	Label  string
	Action func() error
}

// Menu interactivo: lee una línea por stdin y ejecuta la acción.
func Menu(title string, items []MenuItem) error {
	fmt.Println()
	fmt.Println(title)
	for _, it := range items {
		fmt.Printf("  [%s] %s\n", it.Key, it.Label)
	}
	fmt.Println("  [q] salir")
	fmt.Print("> ")
	var choice string
	fmt.Scanln(&choice)
	choice = strings.TrimSpace(strings.ToLower(choice))
	if choice == "q" || choice == "" {
		return nil
	}
	for _, it := range items {
		if strings.ToLower(it.Key) == choice {
			return it.Action()
		}
	}
	fmt.Println("opción inválida:", choice)
	return nil
}
