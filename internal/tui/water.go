package tui

import (
	"math/rand"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Water renders an animated underwater backdrop: a clean sea-colored
// gradient background, a subtle sandy seabed, and a few rising bubbles
// that float BEHIND whatever text is drawn on top of them.
type Water struct {
	width   int
	height  int
	tick    int
	bubbles []bubble
	seabed  int

	// precomputed per-row background styles (light surface -> deep floor)
	rowStyles []lipgloss.Style
	seabedSt  lipgloss.Style
	bubbleSt  lipgloss.Style
}

type bubble struct {
	x, y, speed int
	size        string
}

func NewWater(w, h int) *Water {
	wt := &Water{width: w, height: h, seabed: h - 2}

	// One background style per row for the depth gradient.
	// Surface rows are bright teal, deeper rows sink to navy.
	wt.rowStyles = make([]lipgloss.Style, h)
	for y := 0; y < h; y++ {
		wt.rowStyles[y] = lipgloss.NewStyle().Background(wt.bgFor(y))
	}
	wt.seabedSt = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#e8d5a3")).
		Background(lipgloss.Color("#7a5a2e"))
	wt.bubbleSt = lipgloss.NewStyle().
		Foreground(Bubble).
		Bold(true)

	r := rand.New(rand.NewSource(42))

	// A modest field of bubbles so the scene stays calm and readable.
	for i := 0; i < 25; i++ {
		size := "o"
		switch r.Intn(12) {
		case 0, 1, 2:
			size = "O"
		case 3:
			size = "°"
		case 4:
			size = "•"
		case 5:
			size = "ᵒ"
		}
		wt.bubbles = append(wt.bubbles, bubble{
			x:     r.Intn(w),
			y:     r.Intn(h),
			speed: 1 + r.Intn(2),
			size:  size,
		})
	}
	return wt
}

// bgFor returns the sea background color for a given row:
// bright at the surface (top), deep navy at the floor (bottom).
func (w *Water) bgFor(y int) lipgloss.Color {
	frac := float64(y) / float64(max(1, w.height))
	idx := int(frac * float64(len(waterColors)))
	if idx >= len(waterColors) {
		idx = len(waterColors) - 1
	}
	return waterColors[len(waterColors)-1-idx]
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
	for y := 0; y < w.height; y++ {
		lines[y] = w.block(y, 0, w.width)
	}
	return lines
}

// Suffix renders the water for row y starting at column fromX.
// It lets text be placed on the left while the sea (with bubbles) still
// fills the rest of the row — so bubbles float behind and around the text.
func (w *Water) Suffix(y, fromX int) string {
	if fromX < 0 {
		fromX = 0
	}
	return w.block(y, fromX, w.width)
}

// Seg renders the water for row y between columns fromX (inclusive) and
// toX (exclusive), so panels can carve out a fixed-width band on the right
// while the sea fills the remaining chat columns.
func (w *Water) Seg(y, fromX, toX int) string {
	if fromX < 0 {
		fromX = 0
	}
	if toX > w.width {
		toX = w.width
	}
	if toX <= fromX {
		return ""
	}
	return w.block(y, fromX, toX)
}

func (w *Water) activeMap() map[int]map[int]string {
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
	return active
}

// block renders the water for one row, from column fromX to column toX.
func (w *Water) block(y, fromX, toX int) string {
	if w.width <= 0 {
		return ""
	}
	if toX > w.width {
		toX = w.width
	}
	var sb strings.Builder
	base := w.rowStyles[y]
	seabed := y >= w.seabed
	bubbles := w.activeMap()[y]

	for x := fromX; x < toX; x++ {
		switch {
		case bubbles != nil && bubbles[x] != "":
			sb.WriteString(w.bubbleSt.
				Background(w.bgFor(y)).
				Render(bubbles[x]))
		case seabed && x%3 == 1:
			sb.WriteString(w.seabedSt.Render("."))
		default:
			sb.WriteString(base.Render(" "))
		}
	}
	return sb.String()
}
