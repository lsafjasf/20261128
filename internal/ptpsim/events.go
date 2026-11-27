package ptpsim

import (
	"sort"
	"time"
)

const (
	primaryMaster = "master-primary"
	backupMaster  = "master-backup"
	recorderA     = "recorder-a"
	recorderB     = "recorder-b"
	gmPrimary     = "gm-primary"
	gmBackup      = "gm-backup"
)

func stateAt(cfg Config, at time.Duration) string {
	switch {
	case at < cfg.MasterStop:
		return "locked_master"
	case at < cfg.BackupAnnounce:
		return "holdover_no_master"
	case at < cfg.RelockAt:
		return "acquiring_backup"
	default:
		return "locked_backup"
	}
}

func buildTimeline(cfg Config) []TimelineSegment {
	return []TimelineSegment{
		{Start: 0, End: cfg.MasterStop, State: "locked_master", Detail: "primary master announce/sync/delay exchange"},
		{Start: cfg.MasterStop, End: cfg.BackupAnnounce, State: "holdover_no_master", Detail: "no valid announce; local clock free-runs"},
		{Start: cfg.BackupAnnounce, End: cfg.BackupSync, State: "acquiring_backup", Detail: "backup announce received; waiting for sync"},
		{Start: cfg.BackupSync, End: cfg.RelockAt, State: "acquiring_backup", Detail: "sync and delay request convergence"},
		{Start: cfg.RelockAt, End: cfg.Duration, State: "locked_backup", Detail: "backup locked; resampling compensation resumes"},
	}
}

func buildEvents(cfg Config) []Message {
	var events []Message
	var announceSeq, syncSeq, delaySeq uint32

	for at := time.Duration(0); at < cfg.Duration; at += cfg.AnnounceInterval {
		if at < cfg.MasterStop {
			announceSeq++
			events = append(events, Message{
				At: at, Type: Announce, Source: primaryMaster, Target: "all",
				Sequence: announceSeq, Grandmaster: gmPrimary, Priority1: 9,
				ClockState: stateAt(cfg, at),
			})
		}
		if at >= cfg.BackupAnnounce {
			announceSeq++
			events = append(events, Message{
				At: at, Type: Announce, Source: backupMaster, Target: "all",
				Sequence: announceSeq, Grandmaster: gmBackup, Priority1: 127,
				ClockState: stateAt(cfg, at),
			})
		}
	}

	for at := time.Duration(0); at < cfg.MasterStop; at += cfg.SyncInterval {
		syncSeq++
		events = append(events, Message{
			At: at, Type: Sync, Source: primaryMaster, Target: "all",
			Sequence: syncSeq, Grandmaster: gmPrimary, OriginTime: at,
			ClockState: stateAt(cfg, at),
		})
	}
	for at := cfg.BackupSync; at < cfg.Duration; at += cfg.SyncInterval {
		syncSeq++
		events = append(events, Message{
			At: at, Type: Sync, Source: backupMaster, Target: "all",
			Sequence: syncSeq, Grandmaster: gmBackup, OriginTime: at,
			ClockState: stateAt(cfg, at),
		})
	}

	for at := time.Duration(0); at < cfg.Duration; at += cfg.DelayInterval {
		for _, rec := range []struct {
			id     string
			asym   time.Duration
			source string
		}{
			{recorderA, cfg.PathAsymmetry.A, primaryMaster},
			{recorderB, cfg.PathAsymmetry.B, primaryMaster},
		} {
			delaySeq++
			target := rec.source
			if at >= cfg.BackupSync {
				target = backupMaster
			}
			events = append(events, Message{
				At: at, Type: DelayReq, Source: rec.id, Target: target,
				Sequence: delaySeq, PathAsymmetry: rec.asym,
				ClockState: stateAt(cfg, at),
			})
			respond := at < cfg.MasterStop || at >= cfg.BackupSync
			if !respond {
				continue
			}
			events = append(events, Message{
				At: at + 20*time.Microsecond, Type: DelayResp,
				Source: target, Target: rec.id, Sequence: delaySeq,
				OriginTime: at, Correction: rec.asym / 2,
				PathAsymmetry: rec.asym, ClockState: stateAt(cfg, at),
			})
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].At != events[j].At {
			return events[i].At < events[j].At
		}
		return messageRank(events[i].Type) < messageRank(events[j].Type)
	})
	return events
}

func messageRank(t MessageType) int {
	switch t {
	case Announce:
		return 0
	case Sync:
		return 1
	case DelayReq:
		return 2
	default:
		return 3
	}
}

func countMessages(events []Message) map[MessageType]int {
	counts := make(map[MessageType]int)
	for _, event := range events {
		counts[event.Type]++
	}
	return counts
}
