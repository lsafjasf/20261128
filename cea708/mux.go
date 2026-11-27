package cea708

// DecodedBlock bundles the output of one service block with its service id.
type DecodedBlock struct {
	Service  int
	Tokens   []Token
	Commands []Command
}

// Multiplexer owns one ServiceDecoder per caption service number and feeds
// reassembled packets to the right decoder. Service decoders are created
// lazily the first time a block for that service is seen.
type Multiplexer struct {
	services map[int]*ServiceDecoder
	order    []int
}

// NewMultiplexer creates an empty multiplexer.
func NewMultiplexer() *Multiplexer {
	return &Multiplexer{services: make(map[int]*ServiceDecoder)}
}

// Service returns the decoder for a service number, creating it on first use.
func (m *Multiplexer) Service(number int) *ServiceDecoder {
	if d, ok := m.services[number]; ok {
		return d
	}
	d := NewServiceDecoder(number)
	m.services[number] = d
	m.order = append(m.order, number)
	return d
}

// Services lists the service numbers seen so far, in first-seen order.
func (m *Multiplexer) Services() []int {
	return append([]int(nil), m.order...)
}

// PushPacket reassembles a raw DTVCC packet into service blocks and decodes
// each block on its owning service. The per-block results are returned in
// packet order; the window and table state persists across packets inside the
// individual ServiceDecoders.
func (m *Multiplexer) PushPacket(packet []byte) []DecodedBlock {
	_, blocks := ParsePacket(packet)
	out := make([]DecodedBlock, 0, len(blocks))
	for _, block := range blocks {
		decoder := m.Service(block.Number)
		tokens, commands := decoder.DecodeBlock(block.Data)
		// Copy the slices: the decoder reuses its backing arrays across calls,
		// so returning them directly would let a later block overwrite results
		// a caller is still holding.
		tokens = append([]Token(nil), tokens...)
		commands = append([]Command(nil), commands...)
		out = append(out, DecodedBlock{
			Service:  block.Number,
			Tokens:   tokens,
			Commands: commands,
		})
	}
	return out
}

// Reset returns every tracked service to power-on state.
func (m *Multiplexer) Reset() {
	for _, d := range m.services {
		d.Reset()
	}
}
