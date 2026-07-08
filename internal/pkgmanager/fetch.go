package pkgmanager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Fetcher clones a package's source into destDir and reports the exact commit
// it landed on. destDir must not already exist; the fetcher creates it. The
// interface exists so command orchestration can be tested without a network
// or a git binary (see fakeFetcher in the tests).
type Fetcher interface {
	Fetch(spec Spec, destDir string) (commit string, err error)
}

// GitFetcher fetches by shelling out to the system `git`. It clones over
// https, checks out the requested ref, records HEAD, then deletes the `.git`
// directory so installed packages are plain source trees (smaller, and no
// nested-repo surprises for the consuming project's own VCS).
type GitFetcher struct{}

// Fetch implements Fetcher using `git`.
func (GitFetcher) Fetch(spec Spec, destDir string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git not found on PATH — it is required to fetch packages")
	}
	url := spec.CloneURL()

	// Fast path: shallow clone of a named branch/tag. Falls back to a full
	// clone + checkout when the ref is a commit hash (which --branch rejects)
	// or the shallow clone otherwise fails.
	if spec.Ref != "" {
		if err := validateRef(spec.Ref); err != nil {
			return "", err
		}
		if err := run("git", "clone", "--depth", "1", "--branch", spec.Ref, url, destDir); err != nil {
			_ = os.RemoveAll(destDir)
			if err := run("git", "clone", url, destDir); err != nil {
				return "", fmt.Errorf("git clone %s: %w", url, err)
			}
			if err := run("git", "-C", destDir, "checkout", "--quiet", spec.Ref, "--"); err != nil {
				_ = os.RemoveAll(destDir)
				return "", fmt.Errorf("git checkout %s: %w", spec.Ref, err)
			}
		}
	} else {
		if err := run("git", "clone", "--depth", "1", url, destDir); err != nil {
			_ = os.RemoveAll(destDir)
			return "", fmt.Errorf("git clone %s: %w", url, err)
		}
	}

	commit, err := output("git", "-C", destDir, "rev-parse", "HEAD")
	if err != nil {
		_ = os.RemoveAll(destDir)
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	commit = strings.TrimSpace(commit)

	// Strip VCS metadata so the installed tree is just source.
	if err := os.RemoveAll(filepath.Join(destDir, ".git")); err != nil {
		return "", fmt.Errorf("removing .git from %s: %w", destDir, err)
	}
	return commit, nil
}

// validateRef rejects refs git would parse as command-line options (leading
// '-') and characters outside the branch/tag/commit-hash grammar, so a
// manifest entry can never smuggle flags into the git invocations above.
func validateRef(ref string) error {
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("invalid ref %q: must not begin with '-'", ref)
	}
	for _, r := range ref {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-', r == '/', r == '+', r == '~', r == '^', r == '@':
		default:
			return fmt.Errorf("invalid character %q in ref %q", r, ref)
		}
	}
	return nil
}

// run executes a command, forwarding stderr so git's own diagnostics reach the
// user, and discarding stdout.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// output executes a command and returns its stdout.
func output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return string(out), err
}
