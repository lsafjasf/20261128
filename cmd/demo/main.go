package main

import (
	"fmt"
	"time"

	"github.com/lsafjasf/20261128/subtitle"
)

func main() {
	sec := time.Second
	chapters := []subtitle.Chapter{
		{ID: "ch-2", Title: "第二章", Range: subtitle.Range{Start: 10 * sec, End: 20 * sec}},
		{ID: "ch-8", Title: "第八章", Range: subtitle.Range{Start: 70 * sec, End: 80 * sec}},
	}
	cues := []subtitle.Cue{
		{ID: "c2", ChapterID: "ch-2", Range: subtitle.Range{Start: 15 * sec, End: 16 * sec}, Text: "第二章的第一句"},
		{ID: "c8", ChapterID: "ch-8", Range: subtitle.Range{Start: 75 * sec, End: 76 * sec}, Text: "第八章的最后一句"},
	}
	timeline, err := subtitle.NewTimeline(80*sec, chapters, cues)
	if err != nil {
		panic(err)
	}

	session := subtitle.NewSession(timeline)
	show("播放到第八章", session.Seek(75*sec+500*time.Millisecond))
	show("从第八章跳回第二章", session.Seek(15*sec+500*time.Millisecond))
	show("暂停", session.Pause())
	show("播放并进入无字幕段", session.Play().State, session.Seek(18*sec))

	overlap := []subtitle.Chapter{
		{ID: "ch-8", Title: "第八章候选", Range: subtitle.Range{Start: 70 * sec, End: 80 * sec}},
		{ID: "ch-2", Title: "第二章候选", Range: subtitle.Range{Start: 72 * sec, End: 82 * sec}},
	}
	ambiguousTimeline, err := subtitle.NewTimeline(90*sec, overlap, nil)
	if err != nil {
		panic(err)
	}
	ambiguous := subtitle.NewSession(ambiguousTimeline).Seek(75 * sec)
	show("多个章节候选，等待人工判断", ambiguous)
	manual := subtitle.NewSession(ambiguousTimeline)
	manual.Seek(75 * sec)
	chosen, err := manual.SelectChapter("ch-2")
	if err != nil {
		panic(err)
	}
	show("人工选择第二章", chosen)
}

func show(label string, values ...any) {
	fmt.Println(label)
	for _, value := range values {
		switch typed := value.(type) {
		case subtitle.Snapshot:
			chapter := "none"
			if typed.Chapter != nil {
				chapter = typed.Chapter.Title
			}
			fmt.Printf("  chapter=%s status=%s subtitle=%q candidates=%d revision=%d\n",
				chapter, typed.Status, typed.Subtitle, len(typed.Candidates), typed.Revision)
		default:
			fmt.Printf("  %v\n", value)
		}
	}
}
