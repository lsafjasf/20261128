package fieldcut

// Rational is an exact frame/field rate or timestamp, e.g. 60000/1001.
type Rational struct {
	Num int64
	Den int64
}

func R(num, den int64) Rational {
	if den < 0 {
		num, den = -num, -den
	}
	g := gcd(abs(num), den)
	if g > 1 {
		num, den = num/g, den/g
	}
	return Rational{Num: num, Den: den}
}

func (r Rational) Mul(o Rational) Rational { return R(r.Num*o.Num, r.Den*o.Den) }
func (r Rational) Div(o Rational) Rational { return R(r.Num*o.Den, r.Den*o.Num) }

// Scale multiplies r by an integer count (e.g. field index -> timestamp).
func (r Rational) Scale(n int64) Rational { return R(r.Num*n, r.Den) }

func (r Rational) Equal(o Rational) bool { return r.Num == o.Num && r.Den == o.Den }

func (r Rational) Float() float64 { return float64(r.Num) / float64(r.Den) }

func (r Rational) IsInteger() bool { return r.Num%r.Den == 0 }

func (r Rational) String() string {
	if r.Den == 1 {
		return itoa(r.Num)
	}
	return itoa(r.Num) + "/" + itoa(r.Den)
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
