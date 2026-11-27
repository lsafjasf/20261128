package batchout

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// snapshotOutputs is the staging boundary: every regular file is fsynced and
// hashed before the transaction can move to prepared/committing.
func snapshotOutputs(stageDir string) ([]OutputRecord, error) {
	var outputs []OutputRecord
	dirs := map[string]struct{}{stageDir: {}}
	err := filepath.WalkDir(stageDir, func(full string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if full == stageDir || entry.IsDir() {
			if entry.IsDir() {
				dirs[full] = struct{}{}
			}
			return nil
		}
		rel, err := filepath.Rel(stageDir, full)
		if err != nil {
			return err
		}
		rel, err = cleanRelative(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("batchout: staged output %q is not a regular file", rel)
		}
		if err := syncFile(full); err != nil {
			return err
		}
		size, digest, err := fileDigest(full)
		if err != nil {
			return err
		}
		outputs = append(outputs, OutputRecord{
			Name:   rel,
			Path:   rel,
			Size:   size,
			SHA256: digest,
			Mode:   uint32(info.Mode().Perm()),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	ordered := make([]string, 0, len(dirs))
	for dir := range dirs {
		ordered = append(ordered, dir)
	}
	sort.Slice(ordered, func(i, j int) bool {
		return len(ordered[i]) > len(ordered[j])
	})
	for _, dir := range ordered {
		if err := syncDir(dir); err != nil {
			return nil, err
		}
	}
	return outputs, nil
}

func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
