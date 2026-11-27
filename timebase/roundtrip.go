package timebase

// Stamp 是细帧时刻在粗帧时间基上的无损表示。
//
// 精确粗帧 tick = Tick + Rem/Den，其中在 1001/60000 -> 1001/30000
// 这对时间基下 Den 恒为 2、Rem 只可能是 0 或 1。Rem=1 即半帧，
// 余数被显式保留下来，而不是取整丢弃。
type Stamp struct {
	Tick int64 // 粗帧整数 tick
	Rem  int64 // 子 tick 余数（分子）
	Den  int64 // 子 tick 分母
}

// Encode 把 59.94 细帧 tick 无损编码为粗帧 Stamp，全程整数运算。
func Encode(fineTick int64) Stamp {
	// 粗帧 tick = fineTick/2，半帧余数 = fineTick%2。
	return Stamp{
		Tick: fineTick / 2,
		Rem:  fineTick % 2,
		Den:  2,
	}
}

// Decode 把粗帧 Stamp 还原为 59.94 细帧整数 tick。
// 因为余数被保留，还原结果与最初的细帧 tick 完全相等。
func (s Stamp) Decode() int64 {
	// 一个粗帧恰好等于两个细帧：Tick*2 还原整数部分，
	// Rem/Den 个粗帧等于 Rem*2/Den 个细帧（此时间基下即 Rem 本身）。
	return s.Tick*2 + s.Rem*2/s.Den
}

// RoundTrip 执行一次 59.94 -> 29.97 -> 59.94 的无损往返。
func RoundTrip(fineTick int64) int64 {
	return Encode(fineTick).Decode()
}

// NaiveSnapFloor 是反面做法：换算后直接 floor 成整数粗帧 tick，
// 半帧余数被丢弃，再换回细帧时会被吸附到较早的帧。
func NaiveSnapFloor(fineTick int64) int64 {
	coarse := FloorCoarse(fineTick)
	back := ConvertRat(coarse, Coarse2997, Fine5994)
	return back.Num // 整数
}
