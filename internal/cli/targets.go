package cli

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// resolveTargets picks the file set: explicit args override everything; else
// --staged, --diff, or the default git-tracked listing. Inside a git repo,
// directory args and the default listing both go through git, so .gitignore,
// submodules, and nested worktrees are handled by git itself.
func resolveTargets(dir string, args []string, staged bool, diffRef string, exclude []string) ([]string, error) {
	_, inRepo := gitRoot(dir)

	var paths []string
	var err error
	if len(args) > 0 {
		paths, err = collectArgs(dir, args, inRepo)
	} else {
		paths, err = gitListing(dir, staged, diffRef)
	}
	if err != nil {
		return nil, err
	}

	paths = filterExisting(paths, exclude)
	if inRepo {
		paths, err = filterByAttrs(dir, paths)
		if err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// gitRoot returns the worktree root for dir and whether dir is inside a repo.
func gitRoot(dir string) (string, bool) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// gitListing returns tracked (or staged/diffed) file paths joined under dir.
func gitListing(dir string, staged bool, diffRef string) ([]string, error) {
	var names []string
	var err error
	switch {
	case staged:
		names, err = gitLines(dir, "diff", "--cached", "--name-only")
	case diffRef != "":
		names, err = gitLines(dir, "diff", "--name-only", diffRef)
	default:
		names, err = gitLines(dir, "ls-files")
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, n := range names {
		paths = append(paths, filepath.Join(dir, n))
	}
	return paths, nil
}

// collectArgs expands explicit inputs: a file is kept verbatim (naming it is
// intent, even if gitignored); a directory is listed via git ls-files when in a
// repo (tracked only — submodules, nested worktrees, and ignored files drop out)
// and walked directly otherwise.
func collectArgs(dir string, inputs []string, inRepo bool) ([]string, error) {
	var out []string
	for _, in := range inputs {
		info, err := os.Stat(in)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, in)
			continue
		}
		if inRepo {
			names, err := gitLines(dir, "ls-files", "--", in)
			if err != nil {
				return nil, err
			}
			for _, n := range names {
				out = append(out, filepath.Join(dir, n))
			}
			continue
		}
		files, err := walkDir(in)
		if err != nil {
			return nil, err
		}
		out = append(out, files...)
	}
	return out, nil
}

// walkDir returns every regular file under root, skipping any .git directory.
// Only used outside a git repo, where git ls-files is unavailable.
func walkDir(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

func gitLines(dir string, args ...string) ([]string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// filterByAttrs is filled in by a later task; pass-through for now.
func filterByAttrs(dir string, paths []string) ([]string, error) {
	return paths, nil
}

func filterExisting(paths, exclude []string) []string {
	var out []string
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && !excluded(p, exclude) {
			out = append(out, p)
		}
	}
	return out
}

func excluded(path string, exclude []string) bool {
	base := filepath.Base(path)
	for _, pat := range exclude {
		if ok, _ := filepath.Match(pat, base); ok {
			return true
		}
	}
	return false
}
