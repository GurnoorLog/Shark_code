package tui

import (
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Water renders an animated underwater backdrop: gradient blue water,
// a sandy seabed, and rising bubbles in pixel-ASCII style.
type Water struct {
	width   int
	height  int
	tick    int
	bubbles []bubble
	seabed  int

	// precomputed per-row water styles + shared special styles
	rowStyles []lipgloss.Style
	seabedSt  lipgloss.Style
	bubbleSt  lipgloss.Style
	plankton  lipgloss.Style
}

type bubble struct {
	x, y, speed int
	size        string
}

func NewWater(w, h int) *Water {
	wt := &Water{width: w, height: h, seabed: h - 3}

	// Precompute one background style per row for the depth gradient.
	wt.rowStyles = make([]lipgloss.Style, h)
	for y := 0; y < h; y++ {
		frac := float64(y) / float64(max(1, h))
		idx := int(frac * float64(len(waterColors)))
		if idx >= len(waterColors) {
			idx = len(waterColors) - 1
		}
		wt.rowStyles[y] = lipgloss.NewStyle().Foreground(waterColors[idx])
	}
	wt.seabedSt = lipgloss.NewStyle().Foreground(lipgloss.Color("#8a6d3b"))
	wt.bubbleSt = lipgloss.NewStyle().Foreground(Bubble).Bold(true)
	wt.plankton = lipgloss.NewStyle().Foreground(lipgloss.Color("#2b6a8a"))

	// Seed a field of bubbles at random depths.
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 45; i++ {
		size := "o"
		switch r.Intn(12) {
		case 0, 1, 2:
			size = "O"
		case 3:
			size = "°"
		}
		wt.bubbles = append(wt.bubbles, bubble{
			x:     r.Intn(w),
			y:     r.Intn(h),
			speed: 1 + r.Intn(3),
			size:  size,
		})
	}
	return wt
}

func (w *Water) Tick() {
	w.tick++
	for i := range w.bubbles {
		w.bubbles[i].y -= w.bubbles[i].speed
		if w.bubbles[i].y < 0 {
			w.bubbles[i].y = w.height - 1
			w.bubbles[i].x = rand.Intn(w.width)
		}
	}
}

// Render returns the full underwater backdrop, one styled string per row.
func (w *Water) Render() []string {
	lines := make([]string, w.height)

	// Collect bubble cells by row.
	active := map[int]map[int]string{}
	for _, b := range w.bubbles {
		if b.y < 0 || b.y >= w.height {
			continue
		}
		if active[b.y] == nil {
			active[b.y] = map[int]string{}
		}
		active[b.y][b.x] = b.size
	}

	for y := 0; y < w.height; y++ {
		var sb strings.Builder
		bubbles := active[y]
		base := w.rowStyles[y]
		seabed := y >= w.seabed

		// Emit per-cell segments; bubble cells override the water char.
		last := 0
		flush := func(until int, cell func(x int) string) {
			if until <= last {
				return
			}
			var seg strings.Builder
			for x := last; x < until; x++ {
				seg.WriteString(cell(x))
			}
			sb.WriteString(seg.String())
			last = until
		}

		for x := 0; x < w.width; x++ {
			if ch, ok := bubbles[x]; ok {
				flush(x, func(xx int) string { return base.Render(" ") })
				sb.WriteString(w.bubbleSt.Render(ch))
				last = x + 1
			}
		}
		flush(w.width, func(xx int) string {
			if seabed {
				return w.seabedSt.Render(".")
			}
			if (xx*7+y*13+w.tick*2)%37 == 0 {
				return w.plankton.Render("·")
			}
			return base.Render(" ")
		})

		lines[y] = sb.String()
	}
	return lines
}
