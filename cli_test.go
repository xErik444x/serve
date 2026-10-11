package main

import (
	"io"
	"os"
	"strings"
	"testing"
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

// finishOutput corta la captura: restaura stdout (cierra el pipe de escritura)
// y devuelve lo capturado. Sin esto, io.ReadAll bloquea esperando EOF del pipe.
func finishOutput(read func() string, restore func()) string {
	restore()
	return read()
}

// MenuLoop(r, ...): stats queda fijo hasta Enter/q; q desde stats sale del loop.
func TestMenuLoop_StatsFijoHastaEnter(t *testing.T) {
	input := "s\n\nq\n"
	read, restore := captureStdout(t)

	items := []MenuItem{
		{"s", "ver stats", func() error { fmtPrint("PANEL_DE_STATS\n"); return nil }},
	}
	MenuLoop(strings.NewReader(input), "opciones:", items)

	out := finishOutput(read, restore)
	if !strings.Contains(out, "PANEL_DE_STATS") {
		t.Errorf("stats no se mostró: %q", out)
	}
	if !strings.Contains(out, "volver al menú") {
		t.Errorf("sin hint de volver: %q", out)
	}
	// tras el Enter vuelve a mostrar el menú (el loop siguió vivo)
	if !strings.Contains(out, "opciones:") || !strings.Contains(out, "[s] ver stats") {
		t.Errorf("no volvió al menú tras Enter: %q", out)
	}
}

// q desde el modo stats sale directo del loop (no vuelve al menú).
func TestMenuLoop_QDesdeStatsSale(t *testing.T) {
	input := "s\nq\n"
	read, restore := captureStdout(t)

	MenuLoop(strings.NewReader(input), "opciones:", []MenuItem{
		{"s", "ver stats", func() error { fmtPrint("PANEL\n"); return nil }},
	})

	out := finishOutput(read, restore)
	if !strings.Contains(out, "PANEL") {
		t.Errorf("stats no se mostró: %q", out)
	}
	// el menú se muestra UNA sola vez (no volvió tras el q de stats)
	if n := strings.Count(out, "[s] ver stats"); n != 1 {
		t.Errorf("q desde stats debería salir del loop: menú mostrado %d veces", n)
	}
}

func TestMenuLoop_QSale(t *testing.T) {
	input := "q\n"
	read, restore := captureStdout(t)

	MenuLoop(strings.NewReader(input), "opciones:", []MenuItem{
		{"s", "stats", func() error { return nil }},
	})

	out := finishOutput(read, restore)
	if !strings.Contains(out, "opciones:") {
		t.Errorf("sin menú: %q", out)
	}
	if strings.Count(out, "opciones:") != 1 {
		t.Errorf("q debería salir sin re-dibujar: %q", out)
	}
}

func TestMenuLoop_OpcionInvalida(t *testing.T) {
	input := "z\nq\n"
	read, restore := captureStdout(t)

	MenuLoop(strings.NewReader(input), "opciones:", []MenuItem{
		{"s", "stats", func() error { return nil }},
	})

	out := finishOutput(read, restore)
	if !strings.Contains(out, "inválida") {
		t.Errorf("sin aviso de opción inválida: %q", out)
	}
}

// stdin EOF (pipe cerrada / no TTY) sale limpio sin colgar.
func TestMenuLoop_EOF(t *testing.T) {
	read, restore := captureStdout(t)

	MenuLoop(strings.NewReader(""), "opciones:", nil)
	out := finishOutput(read, restore)
	if !strings.Contains(out, "opciones:") {
		t.Errorf("EOF debería mostrar el menú una vez: %q", out)
	}
}
