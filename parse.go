package sf2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Parse reads a complete SoundFont 2 file from r.
func Parse(r io.ReaderAt, size int64) (*SoundFont, error) {
	root, err := ParseRIFF(r, size)
	if err != nil {
		return nil, err
	}
	if root.ListType != "sfbk" {
		return nil, fmt.Errorf("sf2: unexpected RIFF form %q", root.ListType)
	}
	sf := &SoundFont{}
	if info := root.FindList("INFO"); info != nil {
		parseInfo(sf, info)
	}
	if sdta := root.FindList("sdta"); sdta != nil {
		if smpl := sdta.Find("smpl"); smpl != nil {
			sf.SampleData = make([]int16, len(smpl.Data)/2)
			for i := range sf.SampleData {
				sf.SampleData[i] = int16(binary.LittleEndian.Uint16(smpl.Data[i*2:]))
			}
		}
	}
	pdta := root.FindList("pdta")
	if pdta == nil {
		return nil, fmt.Errorf("sf2: missing pdta chunk")
	}
	if err := parsePdta(sf, pdta); err != nil {
		return nil, err
	}
	return sf, nil
}

func parseInfo(sf *SoundFont, info *Chunk) {
	for _, c := range info.Children {
		switch c.ID {
		case "ifil":
			if len(c.Data) >= 4 {
				sf.Version[0] = binary.LittleEndian.Uint16(c.Data)
				sf.Version[1] = binary.LittleEndian.Uint16(c.Data[2:])
			}
		case "isng":
			sf.SoundEngine = cstr(c.Data)
		case "INAM":
			sf.BankName = cstr(c.Data)
		}
	}
}

func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// fixed-width pdta records
type phdrRec struct {
	name                       string
	preset, bank, bagNdx       uint16
	library, genre, morphology uint32
}

func parsePdta(sf *SoundFont, pdta *Chunk) error {
	phdrs, err := readPhdrs(pdta.Find("phdr"))
	if err != nil {
		return err
	}
	pbags, err := readBags(pdta.Find("pbag"), "pbag")
	if err != nil {
		return err
	}
	pmods, err := readMods(pdta.Find("pmod"), "pmod")
	if err != nil {
		return err
	}
	pgens, err := readGens(pdta.Find("pgen"), "pgen")
	if err != nil {
		return err
	}
	insts, err := readInsts(pdta.Find("inst"))
	if err != nil {
		return err
	}
	ibags, err := readBags(pdta.Find("ibag"), "ibag")
	if err != nil {
		return err
	}
	imods, err := readMods(pdta.Find("imod"), "imod")
	if err != nil {
		return err
	}
	igens, err := readGens(pdta.Find("igen"), "igen")
	if err != nil {
		return err
	}
	samples, err := readShdrs(pdta.Find("shdr"))
	if err != nil {
		return err
	}
	// The SF2 spec mandates terminal records at the end of phdr, pbag,
	// pmod, inst, ibag, imod and shdr; drop them from the model.
	phdrs = dropTerminal(phdrs)
	pbags = dropTerminal(pbags)
	pmods = dropTerminal(pmods)
	insts = dropTerminal(insts)
	ibags = dropTerminal(ibags)
	imods = dropTerminal(imods)
	samples = dropTerminal(samples)
	sf.Samples = samples
	sf.Presets = buildPresets(phdrs, pbags, pmods, pgens)
	sf.Instruments = buildInstruments(insts, ibags, imods, igens)
	return nil
}

func dropTerminal[T any](s []T) []T {
	if len(s) > 0 {
		return s[:len(s)-1]
	}
	return s
}

func need(c *Chunk, name string) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("sf2: missing %s chunk", name)
	}
	return c.Data, nil
}

func readPhdrs(c *Chunk) ([]phdrRec, error) {
	d, err := need(c, "phdr")
	if err != nil {
		return nil, err
	}
	if len(d)%38 != 0 {
		return nil, fmt.Errorf("sf2: phdr size %d not a multiple of 38", len(d))
	}
	out := make([]phdrRec, len(d)/38)
	for i := range out {
		r := d[i*38:]
		out[i] = phdrRec{
			name:       cstr(r[:20]),
			preset:     binary.LittleEndian.Uint16(r[20:]),
			bank:       binary.LittleEndian.Uint16(r[22:]),
			bagNdx:     binary.LittleEndian.Uint16(r[24:]),
			library:    binary.LittleEndian.Uint32(r[26:]),
			genre:      binary.LittleEndian.Uint32(r[30:]),
			morphology: binary.LittleEndian.Uint32(r[34:]),
		}
	}
	return out, nil
}

type bagRec struct{ genNdx, modNdx uint16 }

func readBags(c *Chunk, name string) ([]bagRec, error) {
	d, err := need(c, name)
	if err != nil {
		return nil, err
	}
	if len(d)%4 != 0 {
		return nil, fmt.Errorf("sf2: bag size %d not a multiple of 4", len(d))
	}
	out := make([]bagRec, len(d)/4)
	for i := range out {
		out[i] = bagRec{
			genNdx: binary.LittleEndian.Uint16(d[i*4:]),
			modNdx: binary.LittleEndian.Uint16(d[i*4+2:]),
		}
	}
	return out, nil
}

func readMods(c *Chunk, name string) ([]Modulator, error) {
	d, err := need(c, name)
	if err != nil {
		return nil, err
	}
	if len(d)%10 != 0 {
		return nil, fmt.Errorf("sf2: mod size %d not a multiple of 10", len(d))
	}
	out := make([]Modulator, len(d)/10)
	for i := range out {
		r := d[i*10:]
		out[i] = Modulator{
			SrcOper:       binary.LittleEndian.Uint16(r),
			DestOper:      binary.LittleEndian.Uint16(r[2:]),
			Amount:        int16(binary.LittleEndian.Uint16(r[4:])),
			AmtSrcOper:    binary.LittleEndian.Uint16(r[6:]),
			TransformOper: binary.LittleEndian.Uint16(r[8:]),
		}
	}
	return out, nil
}

func readGens(c *Chunk, name string) ([]Generator, error) {
	d, err := need(c, name)
	if err != nil {
		return nil, err
	}
	if len(d)%4 != 0 {
		return nil, fmt.Errorf("sf2: gen size %d not a multiple of 4", len(d))
	}
	out := make([]Generator, len(d)/4)
	for i := range out {
		out[i] = Generator{
			Op:    binary.LittleEndian.Uint16(d[i*4:]),
			Value: binary.LittleEndian.Uint16(d[i*4+2:]),
		}
	}
	return out, nil
}

type instRec struct {
	name   string
	bagNdx uint16
}

func readInsts(c *Chunk) ([]instRec, error) {
	d, err := need(c, "inst")
	if err != nil {
		return nil, err
	}
	if len(d)%22 != 0 {
		return nil, fmt.Errorf("sf2: inst size %d not a multiple of 22", len(d))
	}
	out := make([]instRec, len(d)/22)
	for i := range out {
		r := d[i*22:]
		out[i] = instRec{
			name:   cstr(r[:20]),
			bagNdx: binary.LittleEndian.Uint16(r[20:]),
		}
	}
	return out, nil
}

func readShdrs(c *Chunk) ([]*Sample, error) {
	d, err := need(c, "shdr")
	if err != nil {
		return nil, err
	}
	if len(d)%46 != 0 {
		return nil, fmt.Errorf("sf2: shdr size %d not a multiple of 46", len(d))
	}
	out := make([]*Sample, len(d)/46)
	for i := range out {
		r := d[i*46:]
		out[i] = &Sample{
			Name:            cstr(r[:20]),
			Start:           binary.LittleEndian.Uint32(r[20:]),
			End:             binary.LittleEndian.Uint32(r[24:]),
			StartLoop:       binary.LittleEndian.Uint32(r[28:]),
			EndLoop:         binary.LittleEndian.Uint32(r[32:]),
			SampleRate:      binary.LittleEndian.Uint32(r[36:]),
			OriginalPitch:   r[40],
			PitchCorrection: int8(r[41]),
			SampleLink:      binary.LittleEndian.Uint16(r[42:]),
			Type:            binary.LittleEndian.Uint16(r[44:]),
		}
	}
	return out, nil
}

// sliceZones cuts bags/gens/mods into zones for one owner. Terminals (the
// final phdr/inst record) are excluded by the caller passing n-1 owners.
func sliceZones(bags []bagRec, gens []Generator, mods []Modulator, start, end int) []*Zone {
	var zones []*Zone
	for b := start; b < end && b < len(bags); b++ {
		gEnd := len(gens)
		if b+1 < len(bags) {
			gEnd = int(bags[b+1].genNdx)
		}
		mEnd := len(mods)
		if b+1 < len(bags) {
			mEnd = int(bags[b+1].modNdx)
		}
		g0, g1 := int(bags[b].genNdx), gEnd
		m0, m1 := int(bags[b].modNdx), mEnd
		if g0 > g1 || g1 > len(gens) || m0 > m1 || m1 > len(mods) {
			continue // corrupt indices; validation reports the gap
		}
		z := &Zone{
			Generators: append([]Generator(nil), gens[g0:g1]...),
			Modulators: append([]Modulator(nil), mods[m0:m1]...),
		}
		zones = append(zones, z)
	}
	return zones
}

func buildPresets(recs []phdrRec, bags []bagRec, mods []Modulator, gens []Generator) []*Preset {
	var out []*Preset
	for i, r := range recs {
		end := len(bags)
		if i+1 < len(recs) {
			end = int(recs[i+1].bagNdx)
		}
		p := &Preset{
			Name:       r.name,
			Number:     r.preset,
			Bank:       r.bank,
			Library:    r.library,
			Genre:      r.genre,
			Morphology: r.morphology,
		}
		p.Zones = sliceZones(bags, gens, mods, int(r.bagNdx), end)
		for i, z := range p.Zones {
			if _, ok := z.Get(GenInstrument); !ok {
				z.Global = i == 0
			}
		}
		out = append(out, p)
	}
	return out
}

func buildInstruments(recs []instRec, bags []bagRec, mods []Modulator, gens []Generator) []*Instrument {
	var out []*Instrument
	for i, r := range recs {
		end := len(bags)
		if i+1 < len(recs) {
			end = int(recs[i+1].bagNdx)
		}
		inst := &Instrument{Name: r.name}
		inst.Zones = sliceZones(bags, gens, mods, int(r.bagNdx), end)
		for i, z := range inst.Zones {
			if _, ok := z.Get(GenSampleID); !ok {
				z.Global = i == 0
			}
		}
		out = append(out, inst)
	}
	return out
}
