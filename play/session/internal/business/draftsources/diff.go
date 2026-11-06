package draftsources

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// MaxDiffBytes bounds one file's diff as answered: past it the diff is cut at
// a line and marked truncated.
const MaxDiffBytes = 8 * 1024

// FileDiff is a file's unified diff (3 lines of context), at most
// MaxDiffBytes; Truncated means it was cut.
type FileDiff struct {
	Path      string
	Diff      string
	Truncated bool
}

// Diff is the bounded unified diff from before to after of the file at
// filePath.
func Diff(filePath string, before string, after string) FileDiff {
	text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        lines(before),
		B:        lines(after),
		FromFile: "a/" + filePath,
		ToFile:   "b/" + filePath,
		Context:  3,
	})
	if err != nil {
		// Writing to a strings.Builder never fails.
		return FileDiff{Path: filePath, Truncated: true}
	}
	if len(text) <= MaxDiffBytes {
		return FileDiff{Path: filePath, Diff: text}
	}
	cut := text[:MaxDiffBytes]
	if end := strings.LastIndexByte(cut, '\n'); end >= 0 {
		cut = cut[:end+1]
	}
	return FileDiff{Path: filePath, Diff: cut, Truncated: true}
}

// lines splits content into lines that each end in a newline, the form the
// diff prints; a last line without one gets it.
func lines(content string) []string {
	split := strings.SplitAfter(content, "\n")
	if split[len(split)-1] == "" {
		split = split[:len(split)-1]
	} else {
		split[len(split)-1] += "\n"
	}
	return split
}
