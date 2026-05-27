package gitpaths

import (
	"sync"
)

// Key uniquely identifies a git clone in the cache.
// Using a struct (rather than a string) prevents accidental key collisions
// from string concatenation of RepoURL, ActiveBranch, and ActivePath.
type Key struct {
	RepoURL      string
	ActiveBranch string
	// ActivePath is empty when activePath is not configured.
	ActivePath string
}

var storage sync.Map

// Get retrieves the path associated with the given key from the storage.
func Get(key Key) string {
	path, ok := storage.Load(key)
	if !ok {
		return ""
	}
	//nolint:forcetypeassert // sync.Map stores string values, type is guaranteed
	return path.(string)
}

// GetValues returns all paths stored in the storage.
func GetValues() []string {
	var values []string
	storage.Range(func(key, path any) bool {
		//nolint:forcetypeassert // sync.Map stores string values, type is guaranteed
		values = append(values, path.(string))
		return true
	})
	return values
}

// Set stores a path for the given key.
func Set(key Key, path string) {
	storage.Store(key, path)
}
