package bakfiles

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Collector struct {
	seen  map[string]bool
	skip  map[string]bool
	files []string
}

func NewCollector(skipDirs ...string) *Collector {
	skip := make(map[string]bool, len(skipDirs))
	for _, dir := range skipDirs {
		skip[dir] = true
	}
	return &Collector{
		seen:  make(map[string]bool),
		skip:  skip,
		files: []string{},
	}
}

func (c *Collector) walkAction(p string, d fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if d.IsDir() {
		if c.skip[d.Name()] {
			return filepath.SkipDir
		}
		return nil
	}
	if strings.HasSuffix(p, ".bak") && !c.seen[p] {
		c.seen[p] = true
		c.files = append(c.files, p)
	}
	return nil
}

func Collect(paths []string, skipDirs ...string) ([]string, error) {
	collector := NewCollector(skipDirs...)

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		if info.IsDir() {
			if err := filepath.WalkDir(path, collector.walkAction); err != nil {
				return nil, err
			}

			continue
		}

		if strings.HasSuffix(path, ".bak") && !collector.seen[path] {
			collector.seen[path] = true
			collector.files = append(collector.files, path)
		}
	}

	sort.Strings(collector.files)

	return collector.files, nil
}
