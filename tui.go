package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---- mensajes Bubble Tea ----

// tickMsg dispara el refresco periódico del dashboard.
type tickMsg time.Time

// logMsg es una línea de log del server (llega por canal, nunca por stdout).
type logMsg string

// errMsg indica que el server murió.
type errMsg struct{ err error }

// ---- estilos ----

var (
	styTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	styBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 2)
	styLabel  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styValue  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	styCyan   = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	styGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	stySel    = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
)

// hyperlink envuelve text en una secuencia OSC 8 para que el terminal lo haga
// clickeable (Windows Terminal, iTerm2, VS Code, kitty...). Los terminales sin
// soporte ignoran los escapes y muestran sólo el texto.
func hyperlink(url, text string) string {
	if url == "" {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// urlClickable renderiza el URL subrayado + hyperlink OSC 8.
func urlClickable(url string) string {
	return hyperlink(url, styCyan.Underline(true).Render(url))
}

// ---- modelo ----

// tuiModel es el estado completo de la TUI. Separar el estado de la vista
// permite testear Update() sin arrancar un terminal real.
type tuiModel struct {
	srv     *Server
	root    string
	url     string
	start   time.Time
	width   int
	height  int
	view    string // "dashboard" | "menu"
	cursor  int
	items   []MenuItem
	logs    []string
	maxLogs int
	spin    spinner.Model
	message string // mensaje transitorio (resultado de una acción)
	quit    bool
	logCh   chan string
}

func newTUIModel(srv *Server, root, url string, items []MenuItem, logCh chan string) tuiModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styCyan
	return tuiModel{
		srv: srv, root: root, url: url, start: time.Now(),
		view: "dashboard", items: items, maxLogs: 6,
		spin: sp, logCh: logCh,
	}
}

// Init arranca el ticker, el spinner y la escucha del canal de logs.
func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(tickCmd(), waitLogCmd(m.logCh), m.spin.Tick)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// waitLogCmd espera una línea del canal de logs y la entrega como mensaje.
func waitLogCmd(ch chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return logMsg(line)
	}
}

// Update procesa los mensajes. Es lógica pura (sin I/O de terminal) → testeable.
func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.message = ""
		return m, tickCmd()
	case logMsg:
		m.logs = append(m.logs, string(msg))
		if len(m.logs) > m.maxLogs {
			m.logs = m.logs[len(m.logs)-m.maxLogs:]
		}
		return m, waitLogCmd(m.logCh)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case errMsg:
		m.quit = true
		return m, tea.Quit
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey resuelve el teclado según la vista activa. Devuelve el modelo y el comando.
func (m tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "q":
		// 'q' sólo sale cuando NO estamos escribiendo en el menú.
		m.quit = true
		return m, tea.Quit
	}

	if m.view == "menu" {
		switch key {
		case "esc", "enter", "backspace":
			m.view = "dashboard"
			m.cursor = 0
			return m, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
			return m, nil
		default:
			// tecla que coincide con el atajo de un ítem
			for i, it := range m.items {
				if strings.EqualFold(it.Key, key) {
					m.cursor = i
					return m.runSelected()
				}
			}
		}
		return m, nil
	}

	// vista dashboard
	switch key {
	case "enter", "m":
		m.view = "menu"
		m.cursor = 0
	case "o":
		if err := OpenBrowser(m.url); err != nil {
			m.message = "no se pudo abrir el navegador: " + err.Error()
		} else {
			m.message = "navegador abierto en " + m.url
		}
	case "l":
		n := listFiles(m.root)
		m.message = fmt.Sprintf("%d archivos (ver /__serve/stats)", n)
	}
	return m, nil
}

// runSelected ejecuta la acción del ítem bajo el cursor.
func (m tuiModel) runSelected() (tea.Model, tea.Cmd) {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return m, nil
	}
	if err := m.items[m.cursor].Action(); err != nil {
		m.message = "error: " + err.Error()
	} else {
		m.message = m.items[m.cursor].Label + " ✓"
	}
	return m, nil
}

// ---- vista ----

// View dibuja la pantalla actual.
func (m tuiModel) View() string {
	if m.quit {
		return ""
	}
	if m.view == "menu" {
		return m.viewMenu()
	}
	return m.viewDashboard()
}

func (m tuiModel) viewDashboard() string {
	h, mi, b, total := m.srv.Stats().Snapshot()
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(h) / float64(total) * 100
	}

	var sb strings.Builder
	sb.WriteString(styTitle.Render(" ⚡ serve-go — dashboard ") + "\n\n")

	// hit rate + barra
	bar := hitBar(hitRate, 24)
	col := styGreen
	if hitRate < 50 {
		col = styRed
	} else if hitRate < 80 {
		col = styYellow
	}
	sb.WriteString("  " + styLabel.Render(pad("cache hit rate", 16)) + col.Render(bar+fmt.Sprintf(" %.1f%%", hitRate)) + "\n\n")

	// contadores
	rows := [][2]string{
		{"url", ""}, // placeholder: el URL se renderiza aparte (hyperlink clickeable)
		{"requests", fmt.Sprint(total)},
		{"hits", fmt.Sprint(h)},
		{"misses", fmt.Sprint(mi)},
		{"bytes enviados", humanBytes(int64(b))},
		{"cache (len)", fmt.Sprintf("%d files", m.srv.cache.Len())},
		{"cache (mem)", humanBytes(m.srv.cache.Bytes())},
		{"root", m.root},
		{"uptime", time.Since(m.start).Round(time.Second).String()},
	}
	for _, r := range rows {
		if r[0] == "url" {
			sb.WriteString("  " + styLabel.Render(pad("url", 16)) + urlClickable(m.url) + "\n")
			continue
		}
		sb.WriteString("  " + styLabel.Render(pad(r[0], 16)) + styValue.Render(r[1]) + "\n")
	}

	// logs recientes
	sb.WriteString("\n  " + styLabel.Render("logs recientes") + "\n")
	if len(m.logs) == 0 {
		sb.WriteString("  " + styDim.Render("(sin requests todavía)") + "\n")
	}
	for _, l := range m.logs {
		sb.WriteString("  " + styDim.Render("› ") + styCyan.Render(l) + "\n")
	}

	if m.message != "" {
		sb.WriteString("\n  " + styYellow.Render(m.message) + "\n")
	}

	sb.WriteString("\n  " + m.spin.View() + styDim.Render(" sirviendo · [Enter] menú · [q] salir"))
	return styBox.Render(sb.String())
}

func (m tuiModel) viewMenu() string {
	var sb strings.Builder
	sb.WriteString(styTitle.Render(" ⚡ serve-go — menú ") + "\n\n")
	for i, it := range m.items {
		cursor := "  "
		label := it.Label
		if i == m.cursor {
			cursor = stySel.Render("▸ ")
			label = stySel.Render(label)
		} else {
			label = styValue.Render(label)
		}
		sb.WriteString(cursor + styLabel.Render("["+it.Key+"] ") + label + "\n")
	}
	if m.message != "" {
		sb.WriteString("\n  " + styYellow.Render(m.message) + "\n")
	}
	sb.WriteString("\n  " + styDim.Render("↑/↓ mover · Enter ejecutar · Esc volver · [q] salir"))
	return styBox.Render(sb.String())
}

// ---- helpers ----

// listFiles recorre root y devuelve la cantidad de archivos.
func listFiles(root string) int {
	n := 0
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// runTUI arranca el programa Bubble Tea en alt-screen.
func runTUI(m tuiModel, errCh <-chan error) error {
	p := tea.NewProgram(m, tea.WithAltScreen())
	// vigila la muerte del server y avisa a la TUI
	go func() {
		if err, ok := <-errCh; ok && err != nil {
			p.Send(errMsg{err})
		}
	}()
	_, err := p.Run()
	return err
}
