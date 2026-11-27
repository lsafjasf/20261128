package sf2

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// testFontBuilder assembles a minimal in-memory SF2.
type testFontBuilder struct {
	presets []testPreset
	insts   []testInst
	samples []testSample
	smplLen int
}

type testPreset struct {
	name  string
	bank  uint16
	num   uint16
	zones [][]Generator // zones[0] without instrument gen = global
}

type testInst struct {
	name  string
	zones [][]Generator // zones[0] without sampleID gen = global
}

type testSample struct {
	name               string
	start, end         uint32
	startLoop, endLoop uint32
	rate               uint32
	pitch              uint8
	link               uint16
	typ                uint16
}

func chunk(id string, payload []byte) []byte {
	var b bytes.Buffer
	b.WriteString(id)
	binary.Write(&b, binary.LittleEndian, uint32(len(payload)))
	b.Write(payload)
	if len(payload)%2 == 1 {
		b.WriteByte(0)
	}
	return b.Bytes()
}

func list(typ string, children ...[]byte) []byte {
	var b bytes.Buffer
	b.WriteString(typ)
	for _, c := range children {
		b.Write(c)
	}
	return chunk("LIST", b.Bytes())
}

func (tf *testFontBuilder) build() []byte {
	var phdr, pbag, pgen, inst, ibag, igen, shdr bytes.Buffer
	var pgenAll, igenAll []Generator
	var pbagRecs, ibagRecs []uint16

	for _, p := range tf.presets {
		name := make([]byte, 20)
		copy(name, p.name)
		phdr.Write(name)
		binary.Write(&phdr, binary.LittleEndian, p.num)
		binary.Write(&phdr, binary.LittleEndian, p.bank)
		binary.Write(&phdr, binary.LittleEndian, uint16(len(pbagRecs)))
		phdr.Write(make([]byte, 12)) // library, genre, morphology
		for _, z := range p.zones {
			pbagRecs = append(pbagRecs, uint16(len(pgenAll)))
			pgenAll = append(pgenAll, z...)
		}
	}
	// terminal phdr
	phdr.Write(make([]byte, 20))
	binary.Write(&phdr, binary.LittleEndian, uint16(0))
	binary.Write(&phdr, binary.LittleEndian, uint16(0))
	binary.Write(&phdr, binary.LittleEndian, uint16(len(pbagRecs)))
	phdr.Write(make([]byte, 12))
	pbagRecs = append(pbagRecs, uint16(len(pgenAll)))
	for _, n := range pbagRecs {
		binary.Write(&pbag, binary.LittleEndian, n)
		binary.Write(&pbag, binary.LittleEndian, uint16(0))
	}
	for _, g := range pgenAll {
		binary.Write(&pgen, binary.LittleEndian, g.Op)
		binary.Write(&pgen, binary.LittleEndian, g.Value)
	}

	for _, in := range tf.insts {
		name := make([]byte, 20)
		copy(name, in.name)
		inst.Write(name)
		binary.Write(&inst, binary.LittleEndian, uint16(len(ibagRecs)))
		for _, z := range in.zones {
			ibagRecs = append(ibagRecs, uint16(len(igenAll)))
			igenAll = append(igenAll, z...)
		}
	}
	inst.Write(make([]byte, 20))
	binary.Write(&inst, binary.LittleEndian, uint16(len(ibagRecs)))
	ibagRecs = append(ibagRecs, uint16(len(igenAll)))
	for _, n := range ibagRecs {
		binary.Write(&ibag, binary.LittleEndian, n)
		binary.Write(&ibag, binary.LittleEndian, uint16(0))
	}
	for _, g := range igenAll {
		binary.Write(&igen, binary.LittleEndian, g.Op)
		binary.Write(&igen, binary.LittleEndian, g.Value)
	}

	for _, s := range tf.samples {
		name := make([]byte, 20)
		copy(name, s.name)
		shdr.Write(name)
		for _, v := range []uint32{s.start, s.end, s.startLoop, s.endLoop, s.rate} {
			binary.Write(&shdr, binary.LittleEndian, v)
		}
		shdr.WriteByte(s.pitch)
		shdr.WriteByte(0)
		binary.Write(&shdr, binary.LittleEndian, s.link)
		binary.Write(&shdr, binary.LittleEndian, s.typ)
	}
	// terminal shdr (EOS)
	shdr.Write(make([]byte, 46))

	smpl := make([]byte, tf.smplLen*2)
	info := list("INFO",
		chunk("ifil", []byte{1, 0, 2, 0}),
		chunk("isng", append([]byte("EMU8000"), 0)),
		chunk("INAM", append([]byte("test bank"), 0)),
	)
	sdta := list("sdta", chunk("smpl", smpl))
	pdta := list("pdta",
		chunk("phdr", phdr.Bytes()),
		chunk("pbag", pbag.Bytes()),
		chunk("pmod", make([]byte, 10)),
		chunk("pgen", pgen.Bytes()),
		chunk("inst", inst.Bytes()),
		chunk("ibag", ibag.Bytes()),
		chunk("imod", make([]byte, 10)),
		chunk("igen", igen.Bytes()),
		chunk("shdr", shdr.Bytes()),
	)
	body := append([]byte("sfbk"), append(info, append(sdta, pdta...)...)...)
	var out bytes.Buffer
	out.WriteString("RIFF")
	binary.Write(&out, binary.LittleEndian, uint32(len(body)))
	out.Write(body)
	return out.Bytes()
}

func parseTestFont(t *testing.T, data []byte) *SoundFont {
	t.Helper()
	sf, err := Parse(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return sf
}
