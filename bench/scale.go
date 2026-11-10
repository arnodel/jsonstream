package main

import (
	"cmp"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// scale runs the suite on a generated file of each size in s.scaleItems and
// draws how each tool's time and peak memory grow with the size of the file.
func scale(s suite, svgPath string, runs int) {
	tools, paths := s.tools()
	sizes := make([]float64, len(s.scaleItems))
	// results[k][w][t] is the result of tool t on workload w with the k-th file.
	results := make([][][]result, len(s.scaleItems))
	for k, n := range s.scaleItems {
		file := filepath.Join(os.TempDir(), fmt.Sprintf("pj-scale-%d.json", n))
		writeGen(file, n)
		info, err := os.Stat(file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		sizes[k] = float64(info.Size()) / 1e6
		fmt.Printf("%d items, %s\n", n, formatMB(sizes[k]))
		results[k] = s.measure(tools, paths, file, runs)
		printResults(tools, s.names(), results[k])
		fmt.Println()
		for _, tool := range tools {
			os.Remove(flatFile(tool, file))
		}
		os.Remove(file)
	}
	if svgPath != "" {
		writeChart(svgPath, scaleSVG(tools, s.names(), sizes, results, runs))
	}
}

// writeGen writes a generated file with n items.
func writeGen(path string, n int) {
	out, err := os.Create(path)
	if err == nil {
		gen(out, n)
		err = out.Close()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// formatMB is a file size given in MB, with two significant digits below 10.
func formatMB(mb float64) string {
	if mb < 10 {
		return fmt.Sprintf("%.2g MB", mb)
	}
	return fmt.Sprintf("%.0f MB", mb)
}

// Layout of the scale chart: a row per workload, each with a time panel and a
// memory panel side by side.
const (
	scalePlotWidth  = 250.0
	scalePlotHeight = 110.0
	scaleAxisLeft   = 44.0 // left of each plot, for the value ticks
	scaleLabelRight = 96.0 // right of each plot, for the tools' end labels
	scaleRowTitle   = 26.0
	scaleAxisBottom = 30.0 // below each plot, for the size ticks
	scaleRowGap     = 16.0
	scaleHeader     = 96.0
	scaleMarker     = 4.0 // radius of the marks at each size
)

// scaleSVG draws, for each workload, how each tool's time and peak memory grow
// with the size of the file: one line per tool.  Both axes are linear, so
// that a tool whose cost grows in proportion to the file draws a straight line.
func scaleSVG(tools, workloads []string, sizes []float64, results [][][]result, runs int) string {
	panelWidth := scaleAxisLeft + scalePlotWidth + scaleLabelRight
	rowHeight := scaleRowTitle + scalePlotHeight + scaleAxisBottom + scaleRowGap
	width := marginX + 2*panelWidth + marginX
	height := scaleHeader + float64(len(workloads))*rowHeight

	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	w(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family='%s' font-size="12">`, width, height, width, height, fontFamily)
	w(`<title>Benchmark: how the time and memory of pj and %s grow with the size of the file</title>`, strings.Join(tools[1:], ", "))
	w(`<rect width="100%%" height="100%%" fill="%s"/>`, surface)

	w(`<text x="%.0f" y="28" fill="%s" font-size="15" font-weight="600">Time and peak memory as the file grows, lower is better</text>`, marginX, inkPrimary)
	w(`<text x="%.0f" y="48" fill="%s">Generated files of %s to %s, output to /dev/null, %s</text>`, marginX, inkSecondary, formatMB(sizes[0]), formatMB(sizes[len(sizes)-1]), runsNote(runs))
	x := marginX
	for _, tool := range tools {
		w(`<rect x="%.0f" y="62" width="10" height="10" rx="2" fill="%s"/>`, x, toolColor(tool))
		w(`<text x="%.0f" y="71" fill="%s">%s</text>`, x+15, inkPrimary, escape(tool))
		x += 15 + 7.5*float64(len(tool)) + 22
	}

	type panel struct {
		title string
		value func(result) float64
		label func(float64) string
	}
	panels := []panel{
		{"Time (seconds)", func(r result) float64 { return r.seconds }, func(v float64) string {
			if v < 0.01 {
				return "< 0.01s"
			}
			return fmt.Sprintf("%.2fs", v)
		}},
		{"Peak memory (MB)", func(r result) float64 { return r.mb }, func(v float64) string {
			return fmt.Sprintf("%.0f MB", v)
		}},
	}
	// The size axis ends at the largest file, with round ticks up to it.
	sizeTop := sizes[len(sizes)-1]
	sizeStep, _ := niceTicks(sizeTop)
	for i, workload := range workloads {
		rowTop := scaleHeader + float64(i)*rowHeight
		w(`<text x="%.0f" y="%.0f" fill="%s" font-weight="600">%s</text>`, marginX, rowTop+16, inkPrimary, escape(workload))
		for p, pn := range panels {
			x0 := marginX + float64(p)*panelWidth + scaleAxisLeft
			y0 := rowTop + scaleRowTitle
			xAt := func(size float64) float64 { return x0 + size/sizeTop*scalePlotWidth }
			maxVal := 0.0
			for k := range sizes {
				for _, r := range results[k][i] {
					if r.ok {
						maxVal = math.Max(maxVal, pn.value(r))
					}
				}
			}
			step, top := niceTicks(maxVal)
			yAt := func(v float64) float64 { return y0 + scalePlotHeight - v/top*scalePlotHeight }

			w(`<text x="%.0f" y="%.0f" fill="%s" font-size="11" text-anchor="end">%s</text>`, x0+scalePlotWidth, rowTop+16, inkSecondary, pn.title)
			for n := 0; float64(n)*step <= top+step/2; n++ {
				v := float64(n) * step
				gy := yAt(v)
				w(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, x0, gy, x0+scalePlotWidth, gy, gridline)
				w(`<text x="%.1f" y="%.1f" fill="%s" font-size="10" text-anchor="end">%s</text>`, x0-6, gy+3.5, inkMuted, formatTick(v))
			}
			w(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, x0, yAt(0), x0+scalePlotWidth, yAt(0), axisLine)
			for n := 0; float64(n)*sizeStep <= sizeTop; n++ {
				v := float64(n) * sizeStep
				label := formatTick(v)
				if v+sizeStep > sizeTop {
					label += " MB"
				}
				w(`<text x="%.1f" y="%.1f" fill="%s" font-size="10" text-anchor="middle">%s</text>`, xAt(v), y0+scalePlotHeight+16, inkMuted, label)
			}

			// One line per tool, broken where a run failed, with a mark at
			// each size. The tools' names and values at the largest size go
			// right of the plot, pushed apart so that they do not overlap.
			type endLabel struct {
				y    float64
				tool int
				text string
			}
			var ends []endLabel
			for t, tool := range tools {
				var pts []string
				flush := func() {
					if len(pts) > 1 {
						w(`<polyline points="%s" fill="none" stroke="%s" stroke-width="2" stroke-linejoin="round"/>`, strings.Join(pts, " "), toolColor(tool))
					}
					pts = pts[:0]
				}
				for k := range sizes {
					r := results[k][i][t]
					if !r.ok {
						flush()
						continue
					}
					pts = append(pts, fmt.Sprintf("%.1f,%.1f", xAt(sizes[k]), yAt(pn.value(r))))
				}
				flush()
				for k, size := range sizes {
					r := results[k][i][t]
					if !r.ok {
						continue
					}
					w(`<circle cx="%.1f" cy="%.1f" r="%.0f" fill="%s" stroke="%s" stroke-width="2"><title>%s, %s: %s</title></circle>`,
						xAt(size), yAt(pn.value(r)), scaleMarker, toolColor(tool), surface, escape(tool), formatMB(size), escape(pn.label(pn.value(r))))
				}
				if last := results[len(sizes)-1][i][t]; last.ok {
					ends = append(ends, endLabel{yAt(pn.value(last)), t, tool + " " + pn.label(pn.value(last))})
				}
			}
			slices.SortStableFunc(ends, func(a, b endLabel) int { return cmp.Compare(a.y, b.y) })
			for j := 1; j < len(ends); j++ {
				ends[j].y = math.Max(ends[j].y, ends[j-1].y+12)
			}
			for _, e := range ends {
				ink, weight := inkSecondary, "normal"
				if e.tool == 0 {
					ink, weight = inkPrimary, "600"
				}
				w(`<text x="%.1f" y="%.1f" fill="%s" font-size="11" font-weight="%s">%s</text>`, xAt(sizeTop)+10, e.y+4, ink, weight, escape(e.text))
			}
		}
	}
	w(`</svg>`)
	return b.String()
}
