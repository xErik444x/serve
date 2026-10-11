package main

import (
	"strings"
	"testing"
	"time"
)

func TestRenderStats_FormatoYColores(t *testing.T) {
	v := StatsView{
		Hits: 13997, Misses: 3, Bytes: 112364000, Total: 14000,
		CacheLen: 1, CacheBytes: 8026, Root: "/tmp/site",
		Uptime: 5 * time.Minute,
	}
	out := RenderStats(v)

	// estructura
	for _, want := range []string{"requests", "hits", "misses", "bytes enviados", "cache (len)", "cache (mem)", "/tmp/site"} {
		if !strings.Contains(out, want) {
			t.Errorf("panel sin %q", want)
		}
	}
	// hit rate 13997/14000 = 99.978 -> "%.1f" redondea a 100.0 (barra casi llena)
	if !strings.Contains(out, "100.0%") {
		t.Errorf("sin hit rate: %q", out)
	}
	// colores ANSI presentes
	for _, code := range []string{cGreen, cYellow, cBlue, cCyan, cBold} {
		if !strings.Contains(out, code) {
			t.Errorf("panel sin color %q", code)
		}
	}
	// barra
	if !strings.Contains(out, "█") || !strings.Contains(out, "░") {
		t.Error("sin barra ascii")
	}
}

func TestRenderStats_NoColor(t *testing.T) {
	colorsOn = false
	t.Cleanup(func() { colorsOn = true })
	out := RenderStats(StatsView{Hits: 1, Misses: 1, Total: 2})
	if strings.Contains(out, cGreen) {
		t.Error("colores deben estar apagados con colorsOn=false")
	}
	if !strings.Contains(out, "50.0%") {
		t.Errorf("hit rate 50%%: %q", out)
	}
}

func TestRenderStats_HitRateColores(t *testing.T) {
	// <50% = rojo, <80% = amarillo, >=80% = verde
	bajo := RenderStats(StatsView{Hits: 1, Misses: 9, Total: 10})
	if !strings.Contains(bajo, cRed) {
		t.Error("hit rate bajo debería ser rojo")
	}
	medio := RenderStats(StatsView{Hits: 7, Misses: 3, Total: 10})
	if !strings.Contains(medio, cYellow) {
		t.Error("hit rate medio debería ser amarillo")
	}
	alto := RenderStats(StatsView{Hits: 9, Misses: 1, Total: 10})
	if !strings.Contains(alto, cGreen) {
		t.Error("hit rate alto debería ser verde")
	}
}

func TestHitBar(t *testing.T) {
	if got := hitBar(100, 10); got != "[██████████]" {
		t.Errorf("100%%: %q", got)
	}
	if got := hitBar(0, 10); got != "[░░░░░░░░░░]" {
		t.Errorf("0%%: %q", got)
	}
	if got := hitBar(50, 4); got != "[██░░]" {
		t.Errorf("50%%: %q", got)
	}
	// fuera de rango no explota
	if got := hitBar(250, 4); got != "[████]" {
		t.Errorf("250%%: %q", got)
	}
	if got := hitBar(-5, 4); got != "[░░░░]" {
		t.Errorf("-5%%: %q", got)
	}
}
