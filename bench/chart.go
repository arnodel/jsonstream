package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// Chart colours and chrome follow a validated categorical palette: four slots in
// a fixed order (blue, orange, aqua, yellow), text in ink tokens rather than
// series colours, hairline solid gridlines, bars at most 24px thick with a 4px
// rounded data end and a 2px gap between neighbours.
var (
	seriesColors = []string{"#2a78d6", "#eb6834", "#1baf7a", "#eda100"}
	surface      = "#fcfcfb"
	inkPrimary   = "#0b0b0b"
	inkSecondary = "#52514e"
	inkMuted     = "#898781"
	gridline     = "#e1e0d9"
	axisLine     = "#c3c2b7"
	fontFamily   = `system-ui, -apple-system, "Segoe UI", sans-serif`
)

const (
	barThickness = 14.0
	barGap       = 2.0
	groupAir     = 22.0
	labelColumn  = 190.0
	panelWidth   = 290.0
	panelGap     = 56.0
	marginX      = 20.0
	headerHeight = 104.0
	axisBand     = 36.0
)

// chartSVG draws two panels of grouped horizontal bars, time and peak memory,
// one group per workload that every tool ran, one bar per tool.
func chartSVG(tools []string, results [][]result, file string) string {
	// Keep the workloads every tool ran.
	var rows []int
	for i := range results {
		all := true
		for _, r := range results[i] {
			all = all && r.ok
		}
		if all {
			rows = append(rows, i)
		}
	}
	n := len(tools)
	groupHeight := float64(n)*barThickness + float64(n-1)*barGap
	rowPitch := groupHeight + groupAir
	plotHeight := float64(len(rows)) * rowPitch
	width := marginX + labelColumn + panelWidth + panelGap + panelWidth + marginX
	height := headerHeight + plotHeight + axisBand

	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	w(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family='%s' font-size="12">`, width, height, width, height, fontFamily)
	w(`<title>Benchmark: pj against %s on a %s file</title>`, strings.Join(tools[1:], ", "), filepath.Base(file))
	w(`<rect width="100%%" height="100%%" fill="%s"/>`, surface)

	// Title, subtitle and legend.
	w(`<text x="%.0f" y="28" fill="%s" font-size="15" font-weight="600">Time and peak memory by workload, lower is better</text>`, marginX, inkPrimary)
	w(`<text x="%.0f" y="48" fill="%s">%s, output to /dev/null, one run each</text>`, marginX, inkSecondary, filepath.Base(file))
	x := marginX
	for t, tool := range tools {
		w(`<rect x="%.0f" y="62" width="10" height="10" rx="2" fill="%s"/>`, x, seriesColors[t%len(seriesColors)])
		w(`<text x="%.0f" y="71" fill="%s">%s</text>`, x+15, inkPrimary, escape(tool))
		x += 15 + 7.5*float64(len(tool)) + 22
	}

	type panel struct {
		title string
		x0    float64
		value func(result) float64
		label func(result) string
	}
	panels := []panel{
		{"Time (seconds)", marginX + labelColumn, func(r result) float64 { return r.seconds }, func(r result) string {
			if r.seconds < 0.01 {
				return "< 0.01s"
			}
			return fmt.Sprintf("%.2fs", r.seconds)
		}},
		{"Peak memory (MB)", marginX + labelColumn + panelWidth + panelGap, func(r result) float64 { return r.mb }, func(r result) string {
			return fmt.Sprintf("%.0f MB", r.mb)
		}},
	}
	plotTop := headerHeight
	for _, p := range panels {
		// Axis scale: nice ticks up to just above the largest value.
		maxVal := 0.0
		for _, i := range rows {
			for _, r := range results[i] {
				maxVal = math.Max(maxVal, p.value(r))
			}
		}
		step, top := niceTicks(maxVal)
		scale := func(v float64) float64 { return p.x0 + v/top*panelWidth }

		w(`<text x="%.0f" y="%.0f" fill="%s" font-weight="600">%s</text>`, p.x0, plotTop-12, inkSecondary, p.title)
		for v := 0.0; v <= top+step/2; v += step {
			gx := scale(v)
			w(`<line x1="%.1f" y1="%.0f" x2="%.1f" y2="%.0f" stroke="%s" stroke-width="1"/>`, gx, plotTop, gx, plotTop+plotHeight, gridline)
			w(`<text x="%.1f" y="%.0f" fill="%s" font-size="11" text-anchor="middle">%s</text>`, gx, plotTop+plotHeight+18, inkMuted, formatTick(v))
		}
		// Baseline on top of the gridlines.
		w(`<line x1="%.1f" y1="%.0f" x2="%.1f" y2="%.0f" stroke="%s" stroke-width="1"/>`, p.x0, plotTop, p.x0, plotTop+plotHeight, axisLine)

		for row, i := range rows {
			gy := plotTop + float64(row)*rowPitch + groupAir/2
			for t, r := range results[i] {
				y := gy + float64(t)*(barThickness+barGap)
				bw := scale(p.value(r)) - p.x0
				if bw > 0 {
					w(`<path d="%s" fill="%s"/>`, barPath(p.x0, y, bw, barThickness, 4), seriesColors[t%len(seriesColors)])
				}
				// Direct labels on pj only: it is the series the chart is about;
				// the other values are on the axis and in the README table.
				if t == 0 {
					w(`<text x="%.1f" y="%.1f" fill="%s" font-size="11">%s</text>`, p.x0+bw+6, y+barThickness-3.5, inkSecondary, escape(p.label(r)))
				}
			}
		}
	}
	// Workload labels, vertically centred on each group.
	for row, i := range rows {
		gy := plotTop + float64(row)*rowPitch + groupAir/2 + groupHeight/2 + 4
		w(`<text x="%.0f" y="%.1f" fill="%s" text-anchor="end">%s</text>`, marginX+labelColumn-14, gy, inkPrimary, escape(workloads[i].name))
	}
	w(`</svg>`)
	return b.String()
}

// barPath is a horizontal bar growing from x with rounded corners at the data
// end only; the baseline end stays square.
func barPath(x, y, w, h, r float64) string {
	if w < r {
		r = w
	}
	return fmt.Sprintf("M%.1f,%.1f h%.1f a%.1f,%.1f 0 0 1 %.1f,%.1f v%.1f a%.1f,%.1f 0 0 1 %.1f,%.1f h%.1f z",
		x, y, w-r, r, r, r, r, h-2*r, r, r, -r, r, -(w - r))
}

// niceTicks returns a round tick step and axis top for a maximum value.
func niceTicks(maxVal float64) (step, top float64) {
	if maxVal <= 0 {
		return 1, 1
	}
	raw := maxVal / 4
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if raw <= m*mag {
			step = m * mag
			break
		}
	}
	top = math.Ceil(maxVal/step) * step
	return step, top
}

func formatTick(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%g", v)
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
