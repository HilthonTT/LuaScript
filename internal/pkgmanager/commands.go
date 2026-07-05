package pkgmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Env carries everything the commands operate on: the project root (where the
// manifest, lockfile, and lua_modules/ live), the Fetcher to clone with, and
// an optional progress logger. A zero Logf means silent.
type Env struct {
	Root    string
	Fetcher Fetcher
	Logf    func(format string, args ...any)
}

func (e *Env) log(format string, args ...any) {
	if e.Logf != nil {
		e.Logf(format, args...)
	}
}

func (e *Env) manifestPath() string { return filepath.Join(e.Root, ManifestName) }
func (e *Env) lockPath() string     { return filepath.Join(e.Root, LockName) }
func (e *Env) moduleDir(name string) string {
	return filepath.Join(e.Root, ModulesDir, name)
}

// Add fetches a single package, installs it under lua_modules/<name>, and
// records it in both the manifest and the lockfile. name may be "" to derive
// it from the spec's last path segment.
func (e *Env) Add(specStr, name string) error {
	spec, err := ParseSpec(specStr)
	if err != nil {
		return err
	}
	if name == "" {
		name = spec.DefaultName()
	}
	if err := validName(name); err != nil {
		return err
	}

	manifest, _, err := LoadManifest(e.manifestPath())
	if err != nil {
		return err
	}
	lock, err := LoadLock(e.lockPath())
	if err != nil {
		return err
	}

	commit, err := e.fetchInto(spec, name)
	if err != nil {
		return err
	}

	manifest.Dependencies[name] = spec.String()
	lock.Packages[name] = LockEntry{
		Source: spec.Source,
		Ref:    spec.Ref,
		URL:    spec.CloneURL(),
		Commit: commit,
	}
	if err := manifest.Save(e.manifestPath()); err != nil {
		return err
	}
	if err := lock.Save(e.lockPath()); err != nil {
		return err
	}
	e.log("added %s as '%s' (%s)", spec.String(), name, shortCommit(commit))
	return nil
}

// Install fetches every dependency listed in the manifest that is not already
// present under lua_modules/. When the lockfile pins a commit for a package,
// that exact commit is checked out (reproducible install); otherwise the
// manifest's ref is resolved and written back to the lock.
func (e *Env) Install() error {
	manifest, found, err := LoadManifest(e.manifestPath())
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no %s in %s — run `luascript pkg add <spec>` first", ManifestName, e.Root)
	}
	lock, err := LoadLock(e.lockPath())
	if err != nil {
		return err
	}

	for _, name := range sortedKeys(manifest.Dependencies) {
		spec, err := ParseSpec(manifest.Dependencies[name])
		if err != nil {
			return fmt.Errorf("dependency %q: %w", name, err)
		}
		if err := validName(name); err != nil {
			return err
		}

		// Already installed → leave it. `remove` then `install`, or `add`,
		// is the way to force a refetch.
		if _, statErr := os.Stat(e.moduleDir(name)); statErr == nil {
			e.log("up to date: %s", name)
			continue
		}

		// Reproducible path: if the lock pins a commit, fetch that exact
		// commit rather than whatever the manifest ref now points to.
		fetchSpec := spec
		if locked, ok := lock.Packages[name]; ok && locked.Commit != "" {
			fetchSpec = Spec{Source: spec.Source, Ref: locked.Commit}
		}

		commit, err := e.fetchInto(fetchSpec, name)
		if err != nil {
			return err
		}
		lock.Packages[name] = LockEntry{
			Source: spec.Source,
			Ref:    spec.Ref,
			URL:    spec.CloneURL(),
			Commit: commit,
		}
		e.log("installed %s (%s)", name, shortCommit(commit))
	}
	return lock.Save(e.lockPath())
}

// Remove deletes the installed package and drops it from the manifest and
// lockfile. Removing something that is not installed still prunes the
// manifest/lock entries (and is not an error).
func (e *Env) Remove(name string) error {
	if err := validName(name); err != nil {
		return err
	}
	manifest, _, err := LoadManifest(e.manifestPath())
	if err != nil {
		return err
	}
	lock, err := LoadLock(e.lockPath())
	if err != nil {
		return err
	}

	if err := os.RemoveAll(e.moduleDir(name)); err != nil {
		return fmt.Errorf("removing %s: %w", e.moduleDir(name), err)
	}
	delete(manifest.Dependencies, name)
	delete(lock.Packages, name)

	if err := manifest.Save(e.manifestPath()); err != nil {
		return err
	}
	if err := lock.Save(e.lockPath()); err != nil {
		return err
	}
	e.log("removed %s", name)
	return nil
}

// fetchInto clones spec into lua_modules/<name>, replacing any existing
// install. It ensures the lua_modules root exists first.
func (e *Env) fetchInto(spec Spec, name string) (string, error) {
	dest := e.moduleDir(name)
	if err := os.MkdirAll(filepath.Join(e.Root, ModulesDir), 0o755); err != nil {
		return "", err
	}
	if err := os.RemoveAll(dest); err != nil {
		return "", fmt.Errorf("clearing %s: %w", dest, err)
	}
	e.log("fetching %s ...", spec.String())
	return e.Fetcher.Fetch(spec, dest)
}

// validName rejects package names that would escape lua_modules/ or collide
// with path syntax. Names are used verbatim as a directory and as the
// `require` key, so they must be a single safe path segment.
func validName(name string) error {
	if name == "" {
		return fmt.Errorf("empty package name")
	}
	if name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid package name %q: must be a single path segment", name)
	}
	return nil
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	if c == "" {
		return "unknown"
	}
	return c
}
