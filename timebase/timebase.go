// Package timebase 在纯整数算术下处理 NTSC 分数时间基之间的换算。
//
// 两个相关时间基（单位均为秒）：
//
//	59.94 fps 细帧: 1001/60000
//	29.97 fps 粗帧: 1001/30000
//
// 所有中间量都表示为约分后的有理数 Rat，不使用 float32/float64，
// 因此不会引入二进制浮点舍入，也不会把半帧悄悄吸附到相邻帧。
package timebase

// Timebase 是以秒为单位、用分数表示的每 tick 时长。
type Timebase struct {
	Num int64 // 分子
	Den int64 // 分母，恒为正
}

var (
	// Fine5994 是 59.94 fps 的每帧时长 1001/60000 秒。
	Fine5994 = Timebase{Num: 1001, Den: 60000}
	// Coarse2997 是 29.97 fps 的每帧时长 1001/30000 秒。
	Coarse2997 = Timebase{Num: 1001, Den: 30000}
)

// Rat 是约分后的整数有理数。
type Rat struct {
	Num int64
	Den int64 // 恒为正
}

// gcd 返回两个非负整数的最大公约数。
func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// reduce 把 num/den 约分为 Rat，分母归一化为正数。
func reduce(num, den int64) Rat {
	if den < 0 {
		num, den = -num, -den
	}
	if num == 0 {
		return Rat{Num: 0, Den: 1}
	}
	g := gcd(abs64(num), den)
	return Rat{Num: num / g, Den: den / g}
}

// ConvertRat 返回 src 时间基下 tick 个整数 tick 在 dst 时间基下对应的 tick 数。
// 结果是精确有理数：dstTick = tick * src.Num * dst.Den / (src.Den * dst.Num)。
func ConvertRat(tick int64, src, dst Timebase) Rat {
	return reduce(
		tick*src.Num*dst.Den,
		src.Den*dst.Num,
	)
}

// IsInteger 报告该有理数是否正好落在目标时间基的整数 tick 上。
func (r Rat) IsInteger() bool { return r.Den == 1 }

// Floor 返回不大于 r 的最大整数。
func (r Rat) Floor() int64 {
	q := r.Num / r.Den
	if r.Num < 0 && r.Num%r.Den != 0 {
		q--
	}
	return q
}

// Ceil 返回不小于 r 的最小整数。
func (r Rat) Ceil() int64 {
	q := r.Num / r.Den
	if r.Num > 0 && r.Num%r.Den != 0 {
		q++
	}
	return q
}

// String 以 a/b 的形式输出，整数时只输出 a。
func (r Rat) String() string {
	if r.Den == 1 {
		return itoa(r.Num)
	}
	return itoa(r.Num) + "/" + itoa(r.Den)
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
