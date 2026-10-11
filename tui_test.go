package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel arma un modelo con un server sobre un dir temporal.
func newTestModel(t *testing.T) tuiModel {
	t.Helper()
	dir := t.TempDir()
	srv := New(Config{Root: dir, Port: 0, Quiet: true})
	items := []MenuItem{
		{"o", "abrir", func() error { return nil }},
		{"c", "limpiar", func() error { return nil }},
	}
	return newTUIModel(srv, dir, "http://localhost:0", items, make(chan string, 8))
}

// El dashboard se dibuja al arrancar con los contadores.
func TestTUI_DashboardRender(t *testing.T) {
	m := newTestModel(t)
	out := m.View()
	for _, want := range []string{"serve-go", "cache hit rate", "requests", "root"} {
		if !strings.Contains(out, want) {
			t.Errorf("el dashboard no muestra %q:\n%s", want, out)
		}
	}
}

// q sale del programa.
func TestTUI_QSale(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !nm.(tuiModel).quit {
		t.Error("q debería marcar quit")
	}
}

// Ctrl+C sale.
func TestTUI_CtrlCSale(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !nm.(tuiModel).quit {
		t.Error("ctrl+c debería marcar quit")
	}
}

// Enter abre el menú; Esc vuelve al dashboard.
func TestTUI_EnterAbreMenu_EscVuelve(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(tuiModel)
	if m.view != "menu" {
		t.Fatalf("Enter debería abrir el menú, view=%q", m.view)
	}
	if !strings.Contains(m.View(), "menú") {
		t.Errorf("la vista menú no se renderiza:\n%s", m.View())
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(tuiModel)
	if m.view != "dashboard" {
		t.Errorf("Esc debería volver al dashboard, view=%q", m.view)
	}
}

// Navegación con flechas en el menú.
func TestTUI_MenuNavegaConFlechas(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(tuiModel)
	if m.cursor != 0 {
		t.Fatalf("cursor inicial = %d", m.cursor)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = nm.(tuiModel)
	if m.cursor != 1 {
		t.Errorf("down debería mover el cursor a 1, got %d", m.cursor)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = nm.(tuiModel)
	if m.cursor != 0 {
		t.Errorf("up debería volver el cursor a 0, got %d", m.cursor)
	}
}

// Ejecutar un ítem del menú por su atajo.
func TestTUI_MenuEjecutaPorAtajo(t *testing.T) {
	m := newTestModel(t)
	ran := false
	m.items[1].Action = func() error { ran = true; return nil }
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(tuiModel)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = nm.(tuiModel)
	if !ran {
		t.Error("la tecla 'c' debería ejecutar la acción del ítem correspondiente")
	}
}

// Los logs del server se acumulan (y se recortan al máximo).
func TestTUI_AcumulaLogs(t *testing.T) {
	m := newTestModel(t)
	m.maxLogs = 3
	for _, l := range []string{"a", "b", "c", "d"} {
		nm, _ := m.Update(logMsg(l))
		m = nm.(tuiModel)
	}
	if len(m.logs) != 3 {
		t.Fatalf("esperaba 3 logs (recortado), got %d", len(m.logs))
	}
	if m.logs[len(m.logs)-1] != "d" {
		t.Errorf("el último log debería ser 'd', got %q", m.logs[len(m.logs)-1])
	}
	if !strings.Contains(m.View(), "d") {
		t.Error("el dashboard debería mostrar los logs recientes")
	}
}

// Un error del server marca quit.
func TestTUI_ErrMarcaQuit(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(errMsg{err: errTest{}})
	if !nm.(tuiModel).quit {
		t.Error("errMsg debería marcar quit")
	}
}

type errTest struct{}

func (errTest) Error() string { return "boom" }
