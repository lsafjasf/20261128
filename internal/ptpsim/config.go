package ptpsim

import "time"

// Config describes a deterministic master-to-backup failover run.
// Durations are wall-clock/PTP time unless explicitly marked as sample time.
type Config struct {
	Name             string
	Duration         time.Duration
	SampleRate       float64
	MasterStop       time.Duration
	BackupAnnounce   time.Duration
	BackupSync       time.Duration
	RelockAt         time.Duration
	AnnounceInterval time.Duration
	SyncInterval     time.Duration
	DelayInterval    time.Duration
	PacketInterval   time.Duration

	ClockPPM struct {
		A float64
		B float64
	}
	PathAsymmetry struct {
		A time.Duration
		B time.Duration
	}
	Resampler struct {
		Enabled      bool
		CatchUpAfter bool
	}
	Reorder ReorderConfig
}

type ReorderConfig struct {
	Enabled    bool
	Start      time.Duration
	End        time.Duration
	Every      int
	SwapWindow int
	Drops      int
	Duplicates int
}

func DefaultConfig() Config {
	return Config{
		Name:             "ptp-master-to-backup",
		Duration:         5 * time.Second,
		SampleRate:       96000,
		MasterStop:       1500 * time.Millisecond,
		BackupAnnounce:   2 * time.Second,
		BackupSync:       2050 * time.Millisecond,
		RelockAt:         3 * time.Second,
		AnnounceInterval: 500 * time.Millisecond,
		SyncInterval:     100 * time.Millisecond,
		DelayInterval:    250 * time.Millisecond,
		PacketInterval:   10 * time.Millisecond,
		ClockPPM: struct {
			A float64
			B float64
		}{A: 120, B: -150},
		PathAsymmetry: struct {
			A time.Duration
			B time.Duration
		}{A: 120 * time.Microsecond, B: 20 * time.Microsecond},
		Resampler: struct {
			Enabled      bool
			CatchUpAfter bool
		}{Enabled: true, CatchUpAfter: false},
		Reorder: ReorderConfig{
			Enabled:    true,
			Start:      1400 * time.Millisecond,
			End:        3200 * time.Millisecond,
			Every:      7,
			SwapWindow: 3,
			Drops:      0,
			Duplicates: 0,
		},
	}
}
