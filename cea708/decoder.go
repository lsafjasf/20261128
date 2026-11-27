package cea708

// MaxServices is the number of caption services addressable in a DTVCC stream.
const MaxServices = 63

// MaxWindows is the number of independent caption windows per service.
const MaxWindows = 8

// defaultDesignation returns the table pages designated into G0..G3 at
// power-on, as mandated by ANSI/CTA-708.
func defaultDesignation() [4]TableID {
	return [4]TableID{
		G0: TableDefaultASCII,
		G1: TableLatin1,
		G2: TableExtendedMisc,
		G3: TableFuture,
	}
}

// ServiceDecoder maintains the caption state for one service number. It owns
// the four designation slots, the current GL/GR invocation, transient single
// shifts, the eight windows and any user-defined glyphs.
type ServiceDecoder struct {
	ServiceNumber int

	// designation[g] is the table currently designated into slot Gg.
	designation [4]TableID

	// gl and gr record which slot is currently invoked into the GL region
	// (bytes 0x20..0x7F) and GR region (bytes 0xA0..0xFF).
	gl GraphicSet
	gr GraphicSet

	// singleShift is >= 0 when an SS2/SS3 is in effect for exactly the next
	// graphic byte; -1 means no single shift is pending.
	singleShift GraphicSet

	windows [MaxWindows]*Window
	current int

	// custom glyphs keyed by (slot, graphic index).
	custom map[customKey]CustomChar

	// Tokens and Commands produced by the most recent DecodeBlock call, in
	// stream order. Window state is updated regardless of these buffers.
	Tokens   []Token
	Commands []Command
}

type customKey struct {
	set   GraphicSet
	index byte
}

// NewServiceDecoder creates a decoder for the given 1-based service number.
func NewServiceDecoder(serviceNumber int) *ServiceDecoder {
	d := &ServiceDecoder{ServiceNumber: serviceNumber, current: -1}
	d.resetState()
	return d
}

func (d *ServiceDecoder) resetState() {
	d.designation = defaultDesignation()
	d.gl = G0
	d.gr = G1
	d.singleShift = -1
	d.current = -1
	for i := range d.windows {
		d.windows[i] = newWindow(i)
	}
	d.custom = make(map[customKey]CustomChar)
	d.Tokens = nil
	d.Commands = nil
}

// Reset implements the C1 RST command: all window content, invocation state
// and downloaded glyphs return to their power-on defaults.
func (d *ServiceDecoder) Reset() {
	d.resetState()
}

// CurrentWindow returns the active window, or nil if none is selected.
func (d *ServiceDecoder) CurrentWindow() *Window {
	if d.current < 0 || d.current >= MaxWindows {
		return nil
	}
	return d.windows[d.current]
}

// Window returns the window with the given id, or nil when id is out of range.
// The window object always exists so its Defined flag can be inspected.
func (d *ServiceDecoder) Window(id int) *Window {
	if id < 0 || id >= MaxWindows {
		return nil
	}
	return d.windows[id]
}

// CustomChar reports the downloaded glyph addressed through slot set at the
// given graphic index, if one is currently installed.
func (d *ServiceDecoder) CustomChar(set GraphicSet, index byte) (CustomChar, bool) {
	c, ok := d.custom[customKey{set, index}]
	return c, ok
}
