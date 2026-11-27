package manifest

// Manifest is the merged project description: a flat registry of media files
// and the clips that reference them.
type Manifest struct {
	Media []Media `json:"media"`
	Clips []Clip  `json:"clips"`
}

// Media describes one physical media file on disk.
type Media struct {
	UID      string `json:"uid"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	HashAlgo string `json:"hashAlgo"`
	Hash     string `json:"hash"`
	ShootDay string `json:"shootDay"`
}

// Clip is one timeline/bin clip entry. Bin + ClipNo is the editing namespace;
// LockedUID/LockedHash record which media file the clip is currently bound to.
type Clip struct {
	ID          string `json:"id"`
	ShootDay    string `json:"shootDay"`
	Bin         string `json:"bin"`
	ClipNo      string `json:"clipNo"`
	LockedUID   string `json:"lockedUID"`
	LockedHash  string `json:"lockedHash"`
	ExpectedUID string `json:"expectedUID,omitempty"`
}
