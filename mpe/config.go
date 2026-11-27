package mpe

// handleRPN tracks CC 101/100 (RPN MSB/LSB) and CC 6/38 (data entry) so it
// can recognize an MPE Configuration Message: RPN 6 on a master channel.
// It returns true when the event was consumed as part of RPN machinery.
func (r *Recorder) handleRPN(ev Event) bool {
	switch ev.CC {
	case 101:
		r.rpnMSB[ev.Channel] = ev.Value
		return true
	case 100:
		r.rpnLSB[ev.Channel] = ev.Value
		return true
	case 6:
		r.dataMSB[ev.Channel] = ev.Value
		r.tryConfig(ev.Channel)
		return true
	case 38:
		r.dataLSB[ev.Channel] = ev.Value
		return true
	}
	return false
}

// tryConfig fires when RPN 6 (MSB 0, LSB 6) is completed by data entry on
// a channel that is (or will become) a zone master.
func (r *Recorder) tryConfig(ch int) {
	if r.rpnMSB[ch] != 0 || r.rpnLSB[ch] != 6 {
		return
	}
	if ch != 0 && ch != 15 {
		return
	}
	members := r.dataMSB[ch]
	if members < 0 {
		members = 0
	}
	if members > 14 {
		members = 14
	}
	r.configure(ch, members)
}

// configure rewrites a zone. ch is the master channel (0 for lower, 15 for
// upper) and n is the number of adjacent member channels it owns.
func (r *Recorder) configure(master, n int) {
	if master == 0 {
		r.lower = Zone{Master: 0, Members: makeRange(1, n)}
		return
	}
	if master == 15 {
		start := 15 - n
		members := []int{}
		for i := 0; i < n; i++ {
			members = append(members, start+i)
		}
		r.upper = Zone{Master: 15, Members: members}
	}
}
