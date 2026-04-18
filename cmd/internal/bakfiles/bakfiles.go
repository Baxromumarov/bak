package bakfiles

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Collect(paths []string, skipDirs ...string) ([]string, error) {
	seen := make(map[string]bool)
	skip := make(map[string]bool, len(skipDirs))
	for _, dir := range skipDirs {
		skip[dir] = true
	}

	var files []string

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		if info.IsDir() {
			err := filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					if skip[d.Name()] {
						return filepath.SkipDir
					}
					return nil
				}
				if strings.HasSuffix(p, ".bak") && !seen[p] {
					seen[p] = true
					files = append(files, p)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}

		if strings.HasSuffix(path, ".bak") && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}

	sort.Strings(files)
	return files, nil
}
