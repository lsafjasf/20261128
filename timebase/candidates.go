package timebase

// Candidate 是半帧在粗帧（29.97）整数 tick 上的一个吸附候选。
type Candidate struct {
	// Tick 是候选的粗帧整数 tick。
	Tick int64
	// Label 为 "floor" 或 "ceil"；半帧恰好等于整数时为 "exact"。
	Label string
	// ErrorFine 是以细帧（59.94，1001/60000 秒）为单位的误差：
	// 候选表示的时刻减去精确时刻。floor=-1，ceil=+1，exact=0。
	ErrorFine int64
}

// Analyze 把一个 59.94 细帧 tick 换算到 29.97 粗帧，
// 返回精确的有理数粗帧 tick 和全部整数候选（含每候选的细帧误差）。
// 偶数细帧 tick 恰好落在粗帧上，只有一个 exact 候选；
// 奇数细帧 tick 落在两个粗帧正中间（半帧），给出 floor/ceil 两个候选。
func Analyze(fineTick int64) (exact Rat, candidates []Candidate) {
	exact = ConvertRat(fineTick, Fine5994, Coarse2997)
	if exact.IsInteger() {
		return exact, []Candidate{{
			Tick:      exact.Num,
			Label:     "exact",
			ErrorFine: 0,
		}}
	}

	// 半帧时精确值为 k+1/2：floor 少一个细帧，ceil 多一个细帧。
	floorTick := exact.Floor()
	ceilTick := exact.Ceil()
	return exact, []Candidate{
		{Tick: floorTick, Label: "floor", ErrorFine: -1},
		{Tick: ceilTick, Label: "ceil", ErrorFine: +1},
	}
}

// CoarseErrorFine 返回把 coarseTick 当作 fineTick 的换算结果时，
// 以细帧为单位的误差（粗帧表示时刻 - 精确时刻）。
// 内部全部是整数乘除：coarseTick*2 - fineTick。
func CoarseErrorFine(fineTick, coarseTick int64) int64 {
	return coarseTick*2 - fineTick
}

// FloorCoarse 是恒向下取整策略，半帧吸附到较早的粗帧。
func FloorCoarse(fineTick int64) int64 {
	r := ConvertRat(fineTick, Fine5994, Coarse2997)
	return r.Floor()
}

// CeilCoarse 是恒向上取整策略，半帧吸附到较晚的粗帧。
func CeilCoarse(fineTick int64) int64 {
	r := ConvertRat(fineTick, Fine5994, Coarse2997)
	return r.Ceil()
}

// CumulativeErrorFine 按给定粗帧选择策略，对一批细帧入点
// 累计以细帧为单位的误差。
func CumulativeErrorFine(fineTicks []int64, choose func(int64) int64) int64 {
	var total int64
	for _, fineTick := range fineTicks {
		total += CoarseErrorFine(fineTick, choose(fineTick))
	}
	return total
}
