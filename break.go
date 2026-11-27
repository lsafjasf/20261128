package subtitlelayout

// cannotEndLine contains opening punctuation. A line may not end immediately
// after one of these runes.
func cannotEndLine(r rune) bool {
	switch r {
	case '(', '[', '{', '（', '［', '｛', '「', '『', '【', '〔', '〈', '《', '“', '‘', '‹', '«':
		return true
	default:
		return false
	}
}

// cannotStartLine contains closing punctuation and marks that should remain
// with the preceding text.
func cannotStartLine(r rune) bool {
	switch r {
	case ',', '.', '，', '。', '、', ';', '；', ':', '：', '!', '！', '?', '？',
		')', ']', '}', '）', '］', '｝', '」', '』', '】', '〕', '〉', '》',
		'”', '’', '›', '»', '…', '—', '～', 'ー':
		return true
	default:
		return false
	}
}

func canBreakBetween(prev, next cluster) bool {
	return !cannotEndLine(prev.lead) && !cannotStartLine(next.lead)
}
