package tui

import (
	"math/rand"
	"sort"
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

	bgColor   []lipgloss.Color
	rowStyles []lipgloss.Style
	seabedSt  lipgloss.Style
	bubbleSt  lipgloss.Style

	bubbleAt [][]bubbleInRow
	lines    []string
}

type bubble struct {
	x, y, speed int
	size        string
}

type bubbleInRow struct {
	x  int
	ch string
}

func NewWater(w, h int) *Water {
	wt := &Water{width: w, height: h, seabed: h - 2}

	wt.bgColor = make([]lipgloss.Color, h)
	wt.rowStyles = make([]lipgloss.Style, h)
	for y := 0; y < h; y++ {
		wt.bgColor[y] = wt.bgFor(y)
		wt.rowStyles[y] = lipgloss.NewStyle().Background(wt.bgColor[y])
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
	wt.rebuild()
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

func (w *Water) rebuild() {
	per := make([]map[int]string, w.height)
	for _, b := range w.bubbles {
		if b.y < 0 || b.y >= w.height {
			continue
		}
		if per[b.y] == nil {
			per[b.y] = map[int]string{}
		}
		per[b.y][b.x] = b.size
	}
	w.bubbleAt = make([][]bubbleInRow, w.height)
	for y, m := range per {
		if len(m) == 0 {
			continue
		}
		row := make([]bubbleInRow, 0, len(m))
		for x, ch := range m {
			row = append(row, bubbleInRow{x: x, ch: ch})
		}
		sort.Slice(row, func(i, j int) bool { return row[i].x < row[j].x })
		w.bubbleAt[y] = row
	}
	w.lines = make([]string, w.height)
	for y := 0; y < w.height; y++ {
		w.lines[y] = w.rowSeg(y, 0, w.width)
	}
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
	w.rebuild()
}

// Render returns the full underwater backdrop, one styled string per row.
func (w *Water) Render() []string {
	if w.lines == nil {
		w.rebuild()
	}
	return append([]string(nil), w.lines...)
}

// Suffix renders the water for row y starting at column fromX.
// It lets text be placed on the left while the sea (with bubbles) still
// fills the rest of the row — so bubbles float behind and around the text.
func (w *Water) Suffix(y, fromX int) string {
	if fromX < 0 {
		fromX = 0
	}
	return w.rowSeg(y, fromX, w.width)
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
	return w.rowSeg(y, fromX, toX)
}

func (w *Water) rowSeg(y, fromX, toX int) string {
	if w.width <= 0 || toX <= fromX {
		return ""
	}
	if fromX < 0 {
		fromX = 0
	}
	if toX > w.width {
		toX = w.width
	}
	if toX <= fromX {
		return ""
	}
	var sb strings.Builder
	base := w.rowStyles[y]
	grain := y >= w.seabed
	bub := w.bubbleAt[y]
	x := fromX
	bi := 0
	for x < toX {
		if bi < len(bub) && bub[bi].x < x {
			bi++
			continue
		}
		if bi < len(bub) && bub[bi].x == x {
			sb.WriteString(w.bubbleSt.Background(w.bgColor[y]).Render(bub[bi].ch))
			x++
			bi++
			continue
		}
		if grain && x%3 == 1 {
			sb.WriteString(w.seabedSt.Render("."))
			x++
			continue
		}
		start := x
		for x < toX && !(grain && x%3 == 1) {
			if bi < len(bub) && bub[bi].x == x {
				break
			}
			x++
		}
		if x > start {
			sb.WriteString(base.Render(strings.Repeat(" ", x-start)))
		}
	}
	return sb.String()
}
