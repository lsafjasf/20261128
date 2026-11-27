package conflict

import "clipmerge/internal/identity"

// Group is one conflict cluster identified across the three identity layers.
type Group struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Severity   string      `json:"severity"`
	Reason     string      `json:"reason"`
	Namespace  string      `json:"namespace,omitempty"` // "bin/clipNo"
	ClipIDs    []string    `json:"clipIds,omitempty"`
	MediaUIDs  []string    `json:"mediaUids,omitempty"`
	UIDs       []string    `json:"uids,omitempty"`
	Hashes     []string    `json:"hashes,omitempty"`
	WrongLocks []WrongLock `json:"wrongLocks,omitempty"`
}

// WrongLock describes one clip bound to the wrong media file, together with
// the resolved rollback target (if any).
type WrongLock struct {
	ClipID      string `json:"clipId"`
	ShootDay    string `json:"shootDay"`
	LockedUID   string `json:"lockedUid"`
	LockedHash  string `json:"lockedHash"`
	CorrectUID  string `json:"correctUid,omitempty"`
	CorrectHash string `json:"correctHash,omitempty"`
	ResolvedBy  string `json:"resolvedBy"` // expectedUID|hashLookup|blocked
	Reason      string `json:"reason"`
}

// Report is the full analysis result.
type Report struct {
	Groups       []Group           `json:"groups"`
	ActualHashes map[string]string `json:"actualHashes,omitempty"`
}

// Conflict kinds.
const (
	KindNamespaceCollision = "NAMESPACE_COLLISION"
	KindFullKeyDuplicate   = "FULLKEY_DUPLICATE"
	KindUIDReuse           = "UID_REUSE"
	KindUIDSplit           = "UID_SPLIT"
	KindHashAlias          = "HASH_ALIAS"
	KindHashDrift          = "HASH_DRIFT"
	KindLockDangling       = "LOCK_DANGLING"
	KindWrongLock          = "WRONG_LOCK"
	KindRollbackBlocked    = "ROLLBACK_BLOCKED"
)

// Severity levels.
const (
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

type detector struct {
	idx       *identity.Index
	mediaRoot string
	actual    map[string]string
	groups    []Group
}
