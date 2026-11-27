package cea708

// Cell is one character position in a caption window. A zero Token indicates
// an empty (background) cell.
type Cell struct {
	Token Token
	Used  bool
}

// Window is the mutable state of one of the eight caption windows.
type Window struct {
	ID      int
	Defined bool
	Visible bool
	Rows    int
	Cols    int
	PenRow  int
	PenCol  int

	// Def captures the parameters of the last DefineWindow command.
	Def WindowDef

	cells [][]Cell
}

func newWindow(id int) *Window {
	return &Window{ID: id, Rows: 0, Cols: 0, PenRow: -1, PenCol: -1}
}

// define (re)allocates the backing grid for the window and resets the pen.
func (w *Window) define(def WindowDef) {
	w.Def = def
	w.Rows = def.RowCount
	w.Cols = def.ColumnCount
	w.Visible = def.Visible
	w.cells = make([][]Cell, w.Rows)
	for r := range w.cells {
		w.cells[r] = make([]Cell, w.Cols)
	}
	w.PenRow = 0
	w.PenCol = 0
	w.Defined = true
}

// Cell returns the cell at row, col and whether that position is in range.
func (w *Window) Cell(row, col int) (Cell, bool) {
	if !w.Defined || row < 0 || row >= w.Rows || col < 0 || col >= w.Cols {
		return Cell{}, false
	}
	return w.cells[row][col], true
}

// Snapshot returns a copy of the current grid, row by row, so callers can read
// window contents without holding decoder state.
func (w *Window) Snapshot() [][]Cell {
	if !w.Defined {
		return nil
	}
	out := make([][]Cell, w.Rows)
	for r := range w.cells {
		out[r] = append([]Cell(nil), w.cells[r]...)
	}
	return out
}

// Text renders the window contents to a string. Empty runs between used cells
// become spaces and each occupied row becomes one line.
func (w *Window) Text() string {
	if !w.Defined {
		return ""
	}
	var runes []rune
	for r := 0; r < w.Rows; r++ {
		lastUsed := -1
		for c := 0; c < w.Cols; c++ {
			if w.cells[r][c].Used {
				lastUsed = c
			}
		}
		if r > 0 {
			runes = append(runes, '\n')
		}
		for c := 0; c <= lastUsed; c++ {
			cell := w.cells[r][c]
			switch {
			case !cell.Used:
				runes = append(runes, ' ')
			case cell.Token.Kind == KindChar:
				runes = append(runes, cell.Token.Rune)
			case cell.Token.Kind == KindTransparentSpace:
				runes = append(runes, ' ')
			case cell.Token.Kind == KindNBTransparentSpace:
				runes = append(runes, '\u00A0')
			case cell.Token.Kind == KindClosedCaption:
				runes = append(runes, '[', 'C', 'C', ']')
			case cell.Token.Kind == KindCustomChar:
				runes = append(runes, objectReplacement)
			case cell.Token.Kind == KindP16:
				runes = append(runes, rune(cell.Token.Value))
			default:
				runes = append(runes, '?')
			}
		}
	}
	return string(runes)
}

const objectReplacement = '\uFFFC'

func (w *Window) clear() {
	for r := range w.cells {
		for c := range w.cells[r] {
			w.cells[r][c] = Cell{}
		}
	}
	w.PenRow = 0
	w.PenCol = 0
}

// write places a token at the pen and advances the pen horizontally, wrapping
// to the next row when the right edge is reached.
func (w *Window) write(tok Token) {
	if !w.Defined {
		return
	}
	if w.PenRow < 0 || w.PenCol < 0 {
		w.PenRow, w.PenCol = 0, 0
	}
	if w.PenRow >= w.Rows {
		w.PenRow = w.Rows - 1
	}
	if w.PenCol >= w.Cols {
		w.PenRow++
		w.PenCol = 0
		if w.PenRow >= w.Rows {
			w.scrollUp()
			w.PenRow = w.Rows - 1
		}
	}
	w.cells[w.PenRow][w.PenCol] = Cell{Token: tok, Used: true}
	w.PenCol++
}

func (w *Window) backspace() {
	if !w.Defined {
		return
	}
	if w.PenCol > 0 {
		w.PenCol--
	} else if w.PenRow > 0 {
		w.PenRow--
		w.PenCol = w.Cols - 1
	}
	w.cells[w.PenRow][w.PenCol] = Cell{}
}

func (w *Window) carriageReturn() {
	if !w.Defined {
		return
	}
	w.PenRow++
	w.PenCol = 0
	if w.PenRow >= w.Rows {
		w.scrollUp()
		w.PenRow = w.Rows - 1
	}
}

// horizontalCarriageReturn erases the current row and returns the pen to its
// start (the HCR control).
func (w *Window) horizontalCarriageReturn() {
	if !w.Defined {
		return
	}
	for c := range w.cells[w.PenRow] {
		w.cells[w.PenRow][c] = Cell{}
	}
	w.PenCol = 0
}

func (w *Window) scrollUp() {
	if w.Rows <= 1 {
		w.clear()
		return
	}
	for r := 1; r < w.Rows; r++ {
		w.cells[r-1] = w.cells[r]
	}
	w.cells[w.Rows-1] = make([]Cell, w.Cols)
}
