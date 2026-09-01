package pkgmanager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Fetcher interface {
	Fetch(spec Spec, destDir string) (commit string, err error)
}

type GitFetcher struct{}

func (GitFetcher) Fetch(spec Spec, destDir string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git not found on PATH — it is required to fetch packages")
	}
	url := spec.CloneURL()

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

	if err := os.RemoveAll(filepath.Join(destDir, ".git")); err != nil {
		return "", fmt.Errorf("removing .git from %s: %w", destDir, err)
	}
	return commit, nil
}

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

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return string(out), err
}
