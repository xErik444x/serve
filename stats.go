package main

import (
	"fmt"
	"strings"
	"time"
)

// ANSI color codes (Windows 10+ los soporta nativo en Windows Terminal y con VT en consolas viejas).
const (
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
	cRed    = "\033[31m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cBlue   = "\033[34m"
	cCyan   = "\033[36m"
	cGray   = "\033[90m"
)

// colorize activa/desactiva colores según el flag --no-color o stdout no TTY.
var colorsOn = true

func paint(code, s string) string {
	if !colorsOn {
		return s
	}
	return code + s + cReset
}

// StatsView es un snapshot con valores ya calculados para el panel.
type StatsView struct {
	Hits, Misses, Bytes, Total uint64
	CacheLen                   int
	CacheBytes                 int64
	Root                       string
	Uptime                     time.Duration
}

// RenderStats arma el panel ASCII de stats con colores.
func RenderStats(v StatsView) string {
	var b strings.Builder
	total := v.Hits + v.Misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(v.Hits) / float64(total) * 100
	}

	b.WriteString("\n")
	b.WriteString(paint(cBold+cCyan, "  ╭──────────────────────────────╮") + "\n")
	b.WriteString(paint(cBold+cCyan, "  │   ⚡ serve-go — stats         │") + "\n")
	b.WriteString(paint(cBold+cCyan, "  ╰──────────────────────────────╯") + "\n")
	b.WriteString("\n")

	// hit rate con barra ascii
	bar := hitBar(hitRate, 24)
	color := cGreen
	if hitRate < 50 {
		color = cRed
	} else if hitRate < 80 {
		color = cYellow
	}
	b.WriteString(paint(cBold, "  cache hit rate")+"  "+paint(color, bar+fmt.Sprintf(" %.1f%%", hitRate))+"\n")
	b.WriteString("\n")

	// contadores
	row := func(label, value string, code string) {
		b.WriteString("  " + paint(cDim, pad(label, 16)) + paint(code, value) + "\n")
	}
	row("requests", fmt.Sprint(v.Total), cBold)
	row("hits", fmt.Sprint(v.Hits), cGreen)
	row("misses", fmt.Sprint(v.Misses), cYellow)
	row("bytes enviados", humanBytes(int64(v.Bytes)), cBlue)
	row("cache (len)", fmt.Sprintf("%d files", v.CacheLen), cCyan)
	row("cache (mem)", humanBytes(v.CacheBytes), cCyan)
	row("root", v.Root, cGray)
	b.WriteString("\n")
	return b.String()
}

// hitBar barra ascii proporcional (24 chars max).
func hitBar(rate float64, width int) string {
	filled := int(rate / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
