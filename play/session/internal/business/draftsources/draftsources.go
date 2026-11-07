// Package draftsources defines what a DRAFT game version's authored sources
// may hold, how a change applies to them, and the starter template a new
// DRAFT begins with.
package draftsources

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/diegobermudez03/playhoot/play/internal/business/scenariodoc"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gamebrief"
)

// Limits of one project's sources, counted after a change applies.
const (
	MaxFiles      = 100
	MaxFileBytes  = 256 * 1024
	MaxTotalBytes = 2 * 1024 * 1024
	maxPathLength = 200
)

// RequiredFiles are the entries every project keeps: the build starts from
// them, so a change may replace them but never delete them.
var RequiredFiles = []string{"game.project.json", "server/game.ts", "client/Game.tsx"}

// Diagnostic codes of a refused change.
const (
	CodeEmptyChange     = "empty_change"
	CodeInvalidPath     = "invalid_path"
	CodePathNotAllowed  = "path_not_allowed"
	CodeDuplicatePath   = "duplicate_path"
	CodeNotFound        = "not_found"
	CodeRequiredFile    = "required_file"
	CodeInvalidUTF8     = "invalid_utf8"
	CodeFileTooLarge    = "file_too_large"
	CodeTooManyFiles    = "too_many_files"
	CodeProjectTooLarge = "project_too_large"
	// CodeInvalidScenario refuses a scenarios/*.json file that is not a
	// playhoot.scenario.v1 document, so saved scenarios stay runnable.
	CodeInvalidScenario = "invalid_scenario"
	// CodeInvalidBrief refuses a brief.json that is not a playhoot.brief.v1
	// document, so the brief an agent reads is always readable.
	CodeInvalidBrief = "invalid_brief"
)

// File is one source file of a revision: its content's SHA-256 (lowercase
// hex) and its size in bytes.
type File struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// SameFiles reports whether a and b hold the same paths with the same
// contents.
func SameFiles(a map[string]File, b map[string]File) bool {
	if len(a) != len(b) {
		return false
	}
	for path, file := range a {
		other, found := b[path]
		if !found || other.SHA256 != file.SHA256 {
			return false
		}
	}
	return true
}

// Write puts Content at Path, adding or replacing that file.
type Write struct {
	Path    string
	Content string
}

// Diagnostic is one reason a change is refused; Path is empty when the
// reason is about the change as a whole. Edit is the index of the edit it is
// about, when it is about one.
type Diagnostic struct {
	Path    string `json:"path,omitempty"`
	Code    string `json:"code"`
	Edit    *int   `json:"edit,omitempty"`
	Message string `json:"message"`
}

// Applied is a change applied to a revision: the next revision's files and
// the content of each file the change wrote, by hash. Diagnostics, when
// any, mean the change is refused and the rest is empty.
type Applied struct {
	Files       map[string]File
	Contents    map[string]string
	Diagnostics []Diagnostic
}

// segmentPattern is what each path segment may hold, so a path is always a
// plain relative path that reads the same everywhere it is used.
var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// Apply applies writes and deletes to the files of current, all or nothing.
func Apply(current map[string]File, writes []Write, deletes []string) Applied {
	var diagnostics []Diagnostic
	refuse := func(filePath, code, message string) {
		diagnostics = append(diagnostics, Diagnostic{Path: filePath, Code: code, Message: message})
	}
	if len(writes) == 0 && len(deletes) == 0 {
		refuse("", CodeEmptyChange, "the change writes and deletes nothing")
	}

	seen := make(map[string]bool, len(writes)+len(deletes))
	claim := func(filePath string) bool {
		if seen[filePath] {
			refuse(filePath, CodeDuplicatePath, "the path appears more than once in the change")
			return false
		}
		seen[filePath] = true
		return true
	}

	next := make(map[string]File, len(current)+len(writes))
	for filePath, file := range current {
		next[filePath] = file
	}
	contents := make(map[string]string, len(writes))
	for _, write := range writes {
		if !checkPath(write.Path, refuse) || !claim(write.Path) {
			continue
		}
		if !utf8.ValidString(write.Content) {
			refuse(write.Path, CodeInvalidUTF8, "the content is not valid UTF-8 text")
			continue
		}
		if len(write.Content) > MaxFileBytes {
			refuse(write.Path, CodeFileTooLarge, fmt.Sprintf("the file has %d bytes; the limit is %d", len(write.Content), MaxFileBytes))
			continue
		}
		if strings.HasPrefix(write.Path, "scenarios/") {
			if _, invalid := scenariodoc.Decode([]byte(write.Content)); len(invalid) > 0 {
				for _, problem := range invalid {
					message := problem.Message
					if problem.Field != "" {
						message = problem.Field + ": " + message
					}
					refuse(write.Path, CodeInvalidScenario, message)
				}
				continue
			}
		}
		if write.Path == gamebrief.Path {
			if _, invalid := gamebrief.Parse([]byte(write.Content)); len(invalid) > 0 {
				for _, problem := range invalid {
					message := problem.Message
					if problem.Field != "" {
						message = problem.Field + ": " + message
					}
					refuse(write.Path, CodeInvalidBrief, message)
				}
				continue
			}
		}
		if write.Path == ConfigFile {
			if message := checkConfig(write.Content); message != "" {
				refuse(write.Path, CodeInvalidConfig, message)
				continue
			}
		}
		if write.Path == ProjectFile {
			_, invalidLanguages := DeclaredLanguages(write.Content)
			_, invalidAssets := DeclaredAssets(write.Content)
			if invalid := append(invalidLanguages, invalidAssets...); len(invalid) > 0 {
				diagnostics = append(diagnostics, invalid...)
				continue
			}
		}
		hash := SHA256Hex(write.Content)
		next[write.Path] = File{SHA256: hash, Size: int64(len(write.Content))}
		contents[hash] = write.Content
	}
	for _, deleted := range deletes {
		if !checkPath(deleted, refuse) || !claim(deleted) {
			continue
		}
		if isRequired(deleted) {
			refuse(deleted, CodeRequiredFile, "this file is required and cannot be deleted")
			continue
		}
		if _, found := current[deleted]; !found {
			refuse(deleted, CodeNotFound, "the project has no such file to delete")
			continue
		}
		delete(next, deleted)
	}

	if len(diagnostics) == 0 {
		if len(next) > MaxFiles {
			refuse("", CodeTooManyFiles, fmt.Sprintf("the project would have %d files; the limit is %d", len(next), MaxFiles))
		}
		var total int64
		for _, file := range next {
			total += file.Size
		}
		if total > MaxTotalBytes {
			refuse("", CodeProjectTooLarge, fmt.Sprintf("the project would have %d bytes; the limit is %d", total, MaxTotalBytes))
		}
		for _, required := range RequiredFiles {
			if _, found := next[required]; !found {
				refuse(required, CodeRequiredFile, "this file is required")
			}
		}
	}
	if len(diagnostics) > 0 {
		return Applied{Diagnostics: diagnostics}
	}
	return Applied{Files: next, Contents: contents}
}

// checkPath reports whether filePath is a valid, allowed source path,
// refusing it otherwise.
func checkPath(filePath string, refuse func(filePath, code, message string)) bool {
	if filePath == "" || len(filePath) > maxPathLength || strings.HasPrefix(filePath, "/") || strings.Contains(filePath, `\`) {
		refuse(filePath, CodeInvalidPath, "paths are relative, use forward slashes and hold at most 200 characters")
		return false
	}
	for _, segment := range strings.Split(filePath, "/") {
		if segment == "." || segment == ".." || !segmentPattern.MatchString(segment) {
			refuse(filePath, CodeInvalidPath, "each path segment holds letters, digits, '_', '-' or '.', and is not '.' or '..'")
			return false
		}
	}
	if !Allowed(filePath) {
		refuse(filePath, CodePathNotAllowed, "allowed: game.project.json, config.json, notes.md, brief.json, .ts/.tsx files under shared/, server/ or client/, and .json files under scenarios/")
		return false
	}
	return true
}

// Allowed reports whether a well-formed relative path is one a project may
// hold.
func Allowed(filePath string) bool {
	switch {
	case filePath == ProjectFile || filePath == ConfigFile || filePath == "notes.md" || filePath == gamebrief.Path:
		return true
	case strings.HasPrefix(filePath, "shared/") || strings.HasPrefix(filePath, "server/") || strings.HasPrefix(filePath, "client/"):
		extension := path.Ext(filePath)
		return extension == ".ts" || extension == ".tsx"
	case strings.HasPrefix(filePath, "scenarios/"):
		return path.Ext(filePath) == ".json"
	default:
		return false
	}
}

func isRequired(filePath string) bool {
	for _, required := range RequiredFiles {
		if filePath == required {
			return true
		}
	}
	return false
}

// SHA256Hex is content's SHA-256 as lowercase hex.
func SHA256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

//go:embed template
var template embed.FS

// Template is the starter project a new DRAFT begins with, sorted by path.
func Template() []Write {
	var writes []Write
	err := fs.WalkDir(template, "template", func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(template, filePath)
		if err != nil {
			return err
		}
		writes = append(writes, Write{Path: strings.TrimPrefix(filePath, "template/"), Content: string(content)})
		return nil
	})
	if err != nil {
		// The template is embedded at build time; failing to read it is a
		// broken binary, not a runtime condition.
		panic(fmt.Sprintf("reading the embedded draft template: %s", err))
	}
	sort.Slice(writes, func(i, j int) bool { return writes[i].Path < writes[j].Path })
	return writes
}
