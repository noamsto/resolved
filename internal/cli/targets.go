package cli

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// resolveTargets picks the file set: explicit args (walked) override everything;
// else --staged, --diff, or the default git-tracked listing.
func resolveTargets(dir string, args []string, staged bool, diffRef string, exclude []string) ([]string, error) {
	if len(args) > 0 {
		return expandPaths(args, exclude)
	}
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
	return filterExisting(paths, exclude), nil
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

// expandPaths walks any directories in inputs and returns regular files,
// dropping anything matching an exclude glob (matched against the base name).
func expandPaths(inputs, exclude []string) ([]string, error) {
	var out []string
	for _, in := range inputs {
		info, err := os.Stat(in)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if !excluded(in, exclude) {
				out = append(out, in)
			}
			continue
		}
		err = filepath.WalkDir(in, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && !excluded(p, exclude) {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
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
