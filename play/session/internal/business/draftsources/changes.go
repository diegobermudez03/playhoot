package draftsources

import "sort"

// FileChanges is what tells one revision's files from an earlier one's:
// paths added, paths whose content changed and paths deleted, each sorted.
type FileChanges struct {
	Added    []string
	Modified []string
	Deleted  []string
}

// Changes compares the files of revision to with those of an earlier
// revision from.
func Changes(from map[string]File, to map[string]File) FileChanges {
	changes := FileChanges{Added: []string{}, Modified: []string{}, Deleted: []string{}}
	for filePath, file := range to {
		earlier, found := from[filePath]
		switch {
		case !found:
			changes.Added = append(changes.Added, filePath)
		case earlier.SHA256 != file.SHA256:
			changes.Modified = append(changes.Modified, filePath)
		}
	}
	for filePath := range from {
		if _, found := to[filePath]; !found {
			changes.Deleted = append(changes.Deleted, filePath)
		}
	}
	sort.Strings(changes.Added)
	sort.Strings(changes.Modified)
	sort.Strings(changes.Deleted)
	return changes
}
