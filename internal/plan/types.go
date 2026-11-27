package plan

// RenameEntry is one deterministic bin/clip rename operation.
type RenameEntry struct {
	ClipID       string `json:"clipId"`
	ShootDay     string `json:"shootDay"`
	OldBin       string `json:"oldBin"`
	OldClipNo    string `json:"oldClipNo"`
	NewBin       string `json:"newBin"`
	NewClipNo    string `json:"newClipNo"`
	Reason       string `json:"reason"`
	ConflictKind string `json:"conflictKind"`
}

// RollbackEntry is one explicit clip lock rollback operation.
type RollbackEntry struct {
	ClipID       string `json:"clipId"`
	ShootDay     string `json:"shootDay"`
	FromUID      string `json:"fromUid"`
	FromHash     string `json:"fromHash"`
	ToUID        string `json:"toUid"`
	ToHash       string `json:"toHash"`
	ResolvedBy   string `json:"resolvedBy"`
	Reason       string `json:"reason"`
	ConflictKind string `json:"conflictKind"`
}

// BlockedEntry records a rollback that needs manual intervention.
type BlockedEntry struct {
	ClipID string `json:"clipId"`
	Reason string `json:"reason"`
}

// Plan is the full remediation plan.
type Plan struct {
	Renames  []RenameEntry   `json:"renames"`
	Rollback []RollbackEntry `json:"rollbacks"`
	Blocked  []BlockedEntry  `json:"blocked"`
}
