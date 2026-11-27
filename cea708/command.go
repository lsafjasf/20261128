package cea708

// CmdKind enumerates the C1 caption commands.
type CmdKind int

const (
	CmdSetCurrentWindow CmdKind = iota
	CmdClearWindows
	CmdDisplayWindows
	CmdHideWindows
	CmdToggleWindows
	CmdDeleteWindows
	CmdDelay
	CmdDelayCancel
	CmdReset
	CmdSetPenAttributes
	CmdSetPenColor
	CmdSetPenLocation
	CmdSetWindowAttributes
	CmdDefineWindow
	// CmdDefineCustomChar installs a user-defined glyph.
	CmdDefineCustomChar
)

// Command is a decoded C1 (or extended C3) control command. Fields that do
// not apply to a given CmdKind keep their zero value. Raw always holds the
// exact source bytes so unknown/reserved commands stay reconstructable.
type Command struct {
	Kind CmdKind

	// Window is the target window id (0..7). Used by SetCurrentWindow and
	// DefineWindow.
	Window int

	// Bitmap carries the 8-bit window mask for the window-group commands.
	Bitmap uint8

	// DelayTenths is the DLY value in tenths of a second.
	DelayTenths int

	// WindowDef holds DefineWindow parameters when Kind == CmdDefineWindow.
	WindowDef WindowDef

	// Custom holds a user-defined glyph when Kind == CmdDefineCustomChar.
	Custom CustomChar

	// PenAttr / PenColor / PenLocation / WindowAttr hold the parameters of
	// the corresponding attribute commands.
	PenAttr     PenAttributes
	PenColor    PenColor
	PenLocation PenLocation
	WindowAttr  WindowAttributes

	// Raw retains the exact encoded bytes of the command.
	Raw []byte
}

// WindowDef is the six-byte parameter payload of a DefineWindow command.
type WindowDef struct {
	Priority         int
	ColumnLock       bool
	RowLock          bool
	Visible          bool
	RelativePosition bool
	AnchorVertical   int
	AnchorHorizontal int
	AnchorPoint      int
	RowCount         int
	ColumnCount      int
	PenStyleID       int
	WindowStyleID    int
}

// PenAttributes mirrors the SPA payload.
type PenAttributes struct {
	TextTag   int
	Offset    int
	PenSize   int
	Italic    bool
	Underline bool
	EdgeType  int
	FontStyle int
}

// PenColor mirrors the SPC payload. Colors are 6-bit CEA-708 palette indices.
type PenColor struct {
	FgColor   int
	FgOpacity int
	BgColor   int
	BgOpacity int
	EdgeColor int
}

// PenLocation mirrors the SPL payload.
type PenLocation struct {
	Row int
	Col int
}

// WindowAttributes mirrors the SWA payload.
type WindowAttributes struct {
	FillColor   int
	FillOpacity int
	BorderColor int
	BorderType  int
	WordWrap    bool
	PrintDir    int
	ScrollDir   int
	Justify     int
	EffectDir   int
	EffectSpeed int
}

// CustomChar is a user-defined glyph. The glyph occupies an anchor cell of
// Rows x Columns pixels; Bitmap is row-major with the high bit of the first
// byte being the leftmost pixel of each row.
type CustomChar struct {
	// Set identifies the graphic slot (G0..G3) the glyph is reachable through.
	Set GraphicSet
	// Index is the graphic code point (e.g. 0x20..0x7F for a G0/G2 slot).
	Index byte
	Rows  int
	Cols  int
	// Bitmap holds the packed bit rows; its length is Rows * ceil(Cols/8).
	Bitmap []byte
}

// Pixel reports whether the pixel at (row, col) is set.
func (c CustomChar) Pixel(row, col int) bool {
	if row < 0 || row >= c.Rows || col < 0 || col >= c.Cols {
		return false
	}
	stride := (c.Cols + 7) / 8
	off := row*stride + col/8
	if off >= len(c.Bitmap) {
		return false
	}
	return c.Bitmap[off]&(0x80>>(uint(col)%8)) != 0
}
