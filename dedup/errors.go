package dedup

import "fmt"

// DuplicateIDError indicates two assets share the same ID.
type DuplicateIDError struct{ ID string }

func (e DuplicateIDError) Error() string {
	return fmt.Sprintf("duplicate asset id: %q", e.ID)
}

func errMasterNotInGroup(id, groupID string) error {
	return fmt.Errorf("asset %q is not a member of group %s", id, groupID)
}

// UnknownAssetError indicates a referenced asset does not exist in the pool.
type UnknownAssetError struct{ ID string }

func (e UnknownAssetError) Error() string {
	return fmt.Sprintf("unknown asset id: %q", e.ID)
}

// NoGroupError indicates the asset has no deduplication group (it is a
// singleton and must not be swapped against anything).
type NoGroupError struct{ ID string }

func (e NoGroupError) Error() string {
	return fmt.Sprintf("asset %q has no deduplication group", e.ID)
}
