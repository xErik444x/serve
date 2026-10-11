package main

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
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

// MenuItem es una opción del menú interactivo (TUI Bubble Tea).
type MenuItem struct {
	Key    string // tecla a apretar
	Label  string
	Action func() error
}
