// Command cea708dump decodes reassembled DTVCC caption channel packets passed
// as hexadecimal bytes on the command line or on stdin. It prints the decoded
// graphic tokens and caption commands for every service block, demonstrating
// the four character sets, table switching and custom glyph handling.
package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"cea708/cea708"
)

func main() {
	var input []byte
	if len(os.Args) > 1 {
		raw := strings.Join(os.Args[1:], "")
		raw = strings.ReplaceAll(raw, " ", "")
		raw = strings.ReplaceAll(raw, "0x", "")
		raw = strings.ReplaceAll(raw, ",", "")
		b, err := hex.DecodeString(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid hex input: %v\n", err)
			os.Exit(1)
		}
		input = b
	} else {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			line = strings.ReplaceAll(line, " ", "")
			line = strings.ReplaceAll(line, "0x", "")
			b, err := hex.DecodeString(line)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid hex line: %v\n", err)
				os.Exit(1)
			}
			input = append(input, b...)
		}
	}

	if len(input) == 0 {
		fmt.Fprintln(os.Stderr, "usage: cea708dump <hex bytes>  (or pipe hex on stdin)")
		os.Exit(2)
	}

	mux := cea708.NewMultiplexer()
	blocks := mux.PushPacket(input)
	for _, block := range blocks {
		fmt.Printf("service %d:\n", block.Service)
		for _, tok := range block.Tokens {
			fmt.Printf("  token %s\n", describeToken(tok))
		}
		for _, cmd := range block.Commands {
			fmt.Printf("  command %s raw=%x\n", describeCommand(cmd), cmd.Raw)
		}
	}
}

func describeToken(tok cea708.Token) string {
	switch tok.Kind {
	case cea708.KindChar:
		return fmt.Sprintf("G%d char %q (index %#02x)", tok.Table, tok.Rune, tok.Value)
	case cea708.KindTransparentSpace:
		return "transparent space"
	case cea708.KindNBTransparentSpace:
		return "non-breaking transparent space"
	case cea708.KindClosedCaption:
		return "[CC] sign"
	case cea708.KindCustomChar:
		return fmt.Sprintf("custom char at G%d index %#02x", tok.Table, tok.Value)
	case cea708.KindP16:
		return fmt.Sprintf("P16 %#04x", tok.Value)
	default:
		return fmt.Sprintf("UNKNOWN raw=%x", tok.Raw)
	}
}

func describeCommand(cmd cea708.Command) string {
	switch cmd.Kind {
	case cea708.CmdSetCurrentWindow:
		return fmt.Sprintf("SetCurrentWindow(%d)", cmd.Window)
	case cea708.CmdDefineWindow:
		return fmt.Sprintf("DefineWindow(%d %dx%d)", cmd.Window, cmd.WindowDef.RowCount, cmd.WindowDef.ColumnCount)
	case cea708.CmdDefineCustomChar:
		return fmt.Sprintf("DefineCustomChar(G%d:%#02x %dx%d)", cmd.Custom.Set, cmd.Custom.Index, cmd.Custom.Rows, cmd.Custom.Cols)
	case cea708.CmdReset:
		return "Reset"
	default:
		return fmt.Sprintf("kind=%d", cmd.Kind)
	}
}
