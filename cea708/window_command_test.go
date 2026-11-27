package cea708

import "testing"

func TestWindowLifecycleAndText(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	d.DecodeBlock([]byte{'H', 'e', 'l', 'l', 'o'})

	w := d.CurrentWindow()
	if w == nil || !w.Defined {
		t.Fatal("window 0 should be defined and current")
	}
	if got := w.Text(); got != "Hello" {
		t.Fatalf("window text = %q, want Hello", got)
	}

	// ClearWindows on bit 0 empties the window but keeps it defined.
	d.DecodeBlock([]byte{c1CLW, 0x01})
	if got := w.Text(); got != "" {
		t.Fatalf("window not cleared: %q", got)
	}
	if !w.Defined {
		t.Fatal("clear must not delete the window")
	}

	// DeleteWindows removes the window and clears the current selection.
	d.DecodeBlock([]byte{c1DLW, 0x01})
	if d.Window(0).Defined || d.CurrentWindow() != nil {
		t.Fatal("window should be deleted and deselected")
	}
}

func TestDisplayHideToggle(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	w := d.Window(0)

	d.DecodeBlock([]byte{c1HDW, 0x01})
	if w.Visible {
		t.Fatal("HideWindows should clear visibility")
	}
	d.DecodeBlock([]byte{c1DSW, 0x01})
	if !w.Visible {
		t.Fatal("DisplayWindows should set visibility")
	}
	d.DecodeBlock([]byte{c1TGW, 0x01})
	if w.Visible {
		t.Fatal("ToggleWindows should flip visibility off")
	}
}

func TestBackspaceAndCarriageReturn(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	d.DecodeBlock([]byte{'A', 'B', c0BS})
	if got := d.CurrentWindow().Text(); got != "A " && got != "A" {
		t.Fatalf("backspace result = %q", got)
	}

	d2 := NewServiceDecoder(2)
	defineWindow0(d2)
	d2.DecodeBlock([]byte{'X', c0CR, 'Y'})
	// Row count is 1, so CR scrolls; Y lands on the single remaining row.
	if got := d2.CurrentWindow().Text(); got != "Y" {
		t.Fatalf("carriage return/scroll result = %q", got)
	}
}

func TestDefineWindowParameters(t *testing.T) {
	d := NewServiceDecoder(1)
	d.DecodeBlock([]byte{
		c1DF3,
		0x30, // row lock (bit4) + visible (bit5)
		0x10, // anchor vertical 16
		0x20, // anchor horizontal 32
		0x23, // anchor 2, row count 4 (0x3+1)
		0x47, // col count 8 (0x07+1)
		0x11, // pen style 1, window style 2
	})
	w := d.Window(3)
	def := w.Def
	if !def.Visible || !def.RowLock {
		t.Fatalf("flags not decoded: %+v", def)
	}
	if def.RowCount != 4 || def.ColumnCount != 8 || def.AnchorPoint != 2 {
		t.Fatalf("geometry wrong: %+v", def)
	}
	if def.PenStyleID != 1 || def.WindowStyleID != 2 {
		t.Fatalf("styles wrong: %+v", def)
	}
}

func TestSetCurrentWindowRejectsUndefined(t *testing.T) {
	d := NewServiceDecoder(1)
	d.DecodeBlock([]byte{c1CW2})
	if d.CurrentWindow() != nil {
		t.Fatal("selecting an undefined window must not make it current")
	}
}

func TestResetClearsStateAndShifts(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	d.DecodeBlock([]byte{c0LS2GL})
	if d.gl != G2 {
		t.Fatal("setup: expected GL locked to G2")
	}
	d.DecodeBlock([]byte{c1RST})
	if d.gl != G0 || d.gr != G1 || d.singleShift != -1 {
		t.Fatalf("reset did not restore invocation state: gl=%v gr=%v ss=%v", d.gl, d.gr, d.singleShift)
	}
	if d.Window(0).Defined {
		t.Fatal("reset must delete windows")
	}
}

func TestHorizontalCarriageReturn(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	d.DecodeBlock([]byte{'A', 'B', c0HCR, 'C'})
	// HCR erases the row and returns the pen to column 0, leaving only "C".
	if got := d.CurrentWindow().Text(); got != "C" {
		t.Fatalf("HCR result = %q, want C", got)
	}
}

func TestWrappingAndScroll(t *testing.T) {
	// A 2-row, 3-column window: the fourth char wraps onto the second row.
	d := NewServiceDecoder(1)
	d.DecodeBlock([]byte{
		c1DF0, 0x00, 0x00, 0x00,
		0x01, // row nibble 1 -> 2 rows (p[3])
		0x02, // column value 2 -> 3 cols (p[4])
		0x00,
		c1CW0,
		'A', 'B', 'C', 'D',
	})
	if got := d.CurrentWindow().Text(); got != "ABC\nD" {
		t.Fatalf("wrap result = %q, want ABC\\nD", got)
	}
}

func TestFormFeedClearsWindow(t *testing.T) {
	d := NewServiceDecoder(1)
	defineWindow0(d)
	d.DecodeBlock([]byte{'A', c0FF, 'B'})
	if got := d.CurrentWindow().Text(); got != "B" {
		t.Fatalf("form feed result = %q, want B", got)
	}
}
