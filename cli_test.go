package main

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func captureStdout(t *testing.T) (func() string, func()) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	// read: lee TODO el pipe (bloquea hasta que w se cierre) y suelta r.
	read := func() string {
		out, _ := io.ReadAll(r)
		r.Close()
		return string(out)
	}
	// restore: solo devuelve stdout y cierra w (EOF para el lector). NO cierra r.
	restore := func() { os.Stdout = old; w.Close() }
	return read, restore
}

func finishOutput(read func() string, restore func()) string {
	restore()
	return read()
}

// drain consume el canal del pump (barrido de las goroutine-leaks en tests).
func drain(ch chan string) {
	for range ch {
	}
}

// LiveDashboard: dibuja el panel al arrancar y devuelve "eof" sin colgar.
func TestLiveDashboard_MuestraPanelAlArrancar(t *testing.T) {
	read, restore := captureStdout(t)
	ch := newPump(strings.NewReader(""))
	got := LiveDashboard(ch, func() string { return "ENCABEZADO\nlinea\n" }, time.Hour)
	out := finishOutput(read, restore)

	if got != "eof" {
		t.Errorf("con input vacío esperaba eof, got %q", got)
	}
	if !strings.Contains(out, "ENCABEZADO") {
		t.Errorf("no dibujó el panel al arrancar: %q", out)
	}
}

func TestLiveDashboard_QApuntaSalir(t *testing.T) {
	read, restore := captureStdout(t)
	ch := newPump(strings.NewReader("q\n"))
	got := LiveDashboard(ch, func() string { return "X\n" }, time.Hour)
	finishOutput(read, restore)
	if got != "quit" {
		t.Errorf("q debería dar quit, got %q", got)
	}
}

func TestLiveDashboard_EnterAbreMenu(t *testing.T) {
	read, restore := captureStdout(t)
	ch := newPump(strings.NewReader("\n"))
	got := LiveDashboard(ch, func() string { return "X\n" }, time.Hour)
	finishOutput(read, restore)
	if got != "menu" {
		t.Errorf("Enter debería abrir el menú, got %q", got)
	}
}

// El panel se refresca solo (varias llamadas a render) mientras corre.
func TestLiveDashboard_RefrescaEnElTiempo(t *testing.T) {
	read, restore := captureStdout(t)
	pr, pw := io.Pipe()
	ch := newPump(pr)
	// inyectar "q" tras 40ms: durante ese lapso deben dispararse ticks (5ms)
	go func() {
		time.Sleep(40 * time.Millisecond)
		pw.Write([]byte("q\n"))
	}()
	var calls int
	got := LiveDashboard(ch, func() string { calls++; return "P\n" }, 5*time.Millisecond)
	finishOutput(read, restore)
	if got != "quit" {
		t.Errorf("esperaba quit, got %q", got)
	}
	if calls < 2 {
		t.Errorf("el panel no se refrescó (render llamado %d veces)", calls)
	}
}

// ---- MenuLoop (opcional) ----

func TestMenuLoop_QSale(t *testing.T) {
	read, restore := captureStdout(t)
	ch := newPump(strings.NewReader("q\n"))
	quit := MenuLoop(ch, "opciones:", []MenuItem{{"o", "abrir", func() error { return nil }}})
	out := finishOutput(read, restore)
	if !quit {
		t.Error("q debería salir (quit=true)")
	}
	if !strings.Contains(out, "opciones:") {
		t.Errorf("sin menú: %q", out)
	}
}

// Enter vuelve al dashboard sin salir del programa.
func TestMenuLoop_EnterVuelveSinSalir(t *testing.T) {
	read, restore := captureStdout(t)
	ch := newPump(strings.NewReader("\n"))
	quit := MenuLoop(ch, "opciones:", []MenuItem{{"o", "abrir", func() error { return nil }}})
	finishOutput(read, restore)
	if quit {
		t.Error("Enter NO debería salir (quit=false), debe volver al dashboard")
	}
}

func TestMenuLoop_EjecutaAccionYVuelve(t *testing.T) {
	read, restore := captureStdout(t)
	ch := newPump(strings.NewReader("o\n\n"))
	ran := false
	// o → ejecuta acción; segundo Enter → vuelve (quit=false)
	quit := MenuLoop(ch, "opciones:", []MenuItem{{"o", "abrir", func() error { ran = true; fmtPrint("ABRI\n"); return nil }}})
	out := finishOutput(read, restore)
	if !ran {
		t.Error("la acción no se ejecutó")
	}
	if quit {
		t.Error("tras la acción + Enter debería volver (quit=false)")
	}
	_ = out
}

func TestMenuLoop_OpcionInvalida(t *testing.T) {
	read, restore := captureStdout(t)
	ch := newPump(strings.NewReader("z\nq\n"))
	quit := MenuLoop(ch, "opciones:", []MenuItem{{"o", "abrir", func() error { return nil }}})
	out := finishOutput(read, restore)
	if !quit {
		t.Error("tras opción inválida + q debería salir")
	}
	if !strings.Contains(out, "inválida") {
		t.Errorf("sin aviso de opción inválida: %q", out)
	}
}

// stdin EOF (no TTY) sale limpio sin colgar.
func TestMenuLoop_EOF(t *testing.T) {
	read, restore := captureStdout(t)
	ch := newPump(strings.NewReader(""))
	quit := MenuLoop(ch, "opciones:", nil)
	out := finishOutput(read, restore)
	if !quit {
		t.Error("EOF debería salir (quit=true)")
	}
	if !strings.Contains(out, "opciones:") {
		t.Errorf("EOF debería mostrar el menú una vez: %q", out)
	}
}
