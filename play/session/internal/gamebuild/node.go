package gamebuild

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
)

// DefaultTimeout bounds one build; a project the size limits allow builds in
// a few seconds.
const DefaultTimeout = 60 * time.Second

// maxOldSpaceMB bounds the Node heap of one build.
const maxOldSpaceMB = 512

// toolchainPackages are the frontend repository's packages a build reads,
// whose sources identify the toolchain's version.
var toolchainPackages = []string{"game-core", "game-server", "game-react", "game-sdk", "game-tools", "protocol"}

// inheritedEnvironment is the only environment a build process gets: what
// Node needs to start on each platform, nothing of the application's.
var inheritedEnvironment = []string{"PATH", "SYSTEMROOT", "TEMP", "TMP", "HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA"}

// Node runs the frontend repository's game tools
// (packages/game-tools/src/cli.ts) as a Node process per build.
type Node struct {
	// root is the frontend repository checkout holding the toolchain.
	root    string
	node    string
	timeout time.Duration

	versionOnce sync.Once
	version     string
	versionErr  error
}

// NewNode returns a Builder running the toolchain in root with the node
// executable on PATH. An empty root is Unconfigured.
func NewNode(root string) Builder {
	if root == "" {
		return Unconfigured{}
	}
	return &Node{root: root, node: "node", timeout: DefaultTimeout}
}

// Version implements Builder: the hash of the toolchain packages' sources
// and the lockfile, read once.
func (n *Node) Version(ctx context.Context) (string, error) {
	n.versionOnce.Do(func() {
		n.version, n.versionErr = n.hashToolchain()
	})
	return n.version, n.versionErr
}

func (n *Node) hashToolchain() (string, error) {
	var paths []string
	for _, pkg := range toolchainPackages {
		dir := filepath.Join(n.root, "packages", pkg, "src")
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && !strings.Contains(entry.Name(), ".test.") {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("%w: reading %s: %v", ErrUnavailable, dir, err)
		}
	}
	paths = append(paths, filepath.Join(n.root, "package-lock.json"))
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("%w: reading %s: %v", ErrUnavailable, path, err)
		}
		relative, _ := filepath.Rel(n.root, path)
		fmt.Fprintf(hash, "%s\x00%d\x00", filepath.ToSlash(relative), len(content))
		hash.Write(content)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// cliResult is the game tools' --json output.
type cliResult struct {
	OK          bool `json:"ok"`
	Diagnostics []struct {
		Stage   string `json:"stage"`
		Message string `json:"message"`
		File    string `json:"file"`
		Line    int    `json:"line"`
		Column  int    `json:"column"`
	} `json:"diagnostics"`
	Record struct {
		AspectRatio       string   `json:"aspectRatio"`
		Languages         []string `json:"languages"`
		InputRequirements *struct {
			UsesKeyboard     bool `json:"usesKeyboard"`
			UsesPointerHover bool `json:"usesPointerHover"`
			UsesPointerPress bool `json:"usesPointerPress"`
		} `json:"inputRequirements"`
	} `json:"record"`
}

// Build implements Builder.
func (n *Node) Build(ctx context.Context, input Input) (Output, error) {
	if _, err := n.Version(ctx); err != nil {
		return Output{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()
	out := filepath.Join(input.Dir, "dist")
	cli := filepath.Join(n.root, "packages", "game-tools", "src", "cli.ts")
	command := exec.CommandContext(ctx, n.node,
		"--experimental-strip-types", "--no-warnings=ExperimentalWarning",
		fmt.Sprintf("--max-old-space-size=%d", maxOldSpaceMB),
		cli, "build", input.Dir, "--out", out, "--json")
	command.Dir = n.root
	command.Env = buildEnvironment()
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	runErr := command.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Output{}, context.DeadlineExceeded
	}
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return Output{}, fmt.Errorf("%w: starting node: %v", ErrUnavailable, runErr)
	}

	var result cliResult
	if err := json.Unmarshal(lastLine(stdout.Bytes()), &result); err != nil {
		return Output{}, fmt.Errorf("the game tools answered no result (exit %v): %s", runErr, truncate(stderr.String(), 2000))
	}
	if !result.OK {
		if len(result.Diagnostics) == 0 {
			return Output{}, fmt.Errorf("the game tools failed without diagnostics: %s", truncate(stderr.String(), 2000))
		}
		diagnostics := make([]build.Diagnostic, len(result.Diagnostics))
		for i, d := range result.Diagnostics {
			diagnostics[i] = build.Diagnostic{Stage: build.Stage(d.Stage), File: d.File, Line: d.Line, Column: d.Column, Message: d.Message}
		}
		return Output{Diagnostics: diagnostics}, nil
	}
	server, err := os.ReadFile(filepath.Join(out, "server.js"))
	if err != nil {
		return Output{}, fmt.Errorf("reading the built server.js: %w", err)
	}
	client, err := os.ReadFile(filepath.Join(out, "client.html"))
	if err != nil {
		return Output{}, fmt.Errorf("reading the built client.html: %w", err)
	}
	manifest, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		return Output{}, fmt.Errorf("reading the built manifest.json: %w", err)
	}
	requirements := result.Record.InputRequirements
	if requirements == nil || len(result.Record.Languages) == 0 {
		return Output{}, fmt.Errorf("the game tools answered no input requirements or languages: the toolchain is older than this backend")
	}
	return Output{
		OK: true, Server: server, Client: client, Manifest: manifest, AspectRatio: result.Record.AspectRatio,
		Languages: result.Record.Languages,
		InputRequirements: InputRequirements{
			UsesKeyboard:     requirements.UsesKeyboard,
			UsesPointerHover: requirements.UsesPointerHover,
			UsesPointerPress: requirements.UsesPointerPress,
		},
	}, nil
}

func buildEnvironment() []string {
	var environment []string
	for _, name := range inheritedEnvironment {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

// lastLine is the last non-empty line of output, where --json prints its
// result.
func lastLine(output []byte) []byte {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	return lines[len(lines)-1]
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
