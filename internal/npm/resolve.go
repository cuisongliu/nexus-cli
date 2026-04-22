package npm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func CollectResolvedURLs(sourcePath string) ([]string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("stat source path: %w", err)
	}

	if info.IsDir() {
		return collectNodeModulesURLs(sourcePath)
	}

	name := filepath.Base(sourcePath)
	switch {
	case name == "package-lock.json":
		return collectPackageLockURLs(sourcePath)
	case strings.HasSuffix(name, "yarn.lock"):
		return collectYarnLockURLs(sourcePath)
	default:
		return nil, fmt.Errorf("unsupported file type: %s", sourcePath)
	}
}

func collectPackageLockURLs(packageLockPath string) ([]string, error) {
	file, err := os.Open(packageLockPath)
	if err != nil {
		return nil, fmt.Errorf("open package-lock.json: %w", err)
	}
	defer file.Close()

	var payload any
	if err := json.NewDecoder(file).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode package-lock.json: %w", err)
	}

	var urls []string
	searchResolved(payload, &urls)
	return uniqueSorted(urls), nil
}

func collectYarnLockURLs(yarnLockPath string) ([]string, error) {
	file, err := os.Open(yarnLockPath)
	if err != nil {
		return nil, fmt.Errorf("open yarn.lock: %w", err)
	}
	defer file.Close()

	urls, err := parseYarnResolvedURLs(file)
	if err != nil {
		return nil, err
	}
	return uniqueSorted(urls), nil
}

func collectNodeModulesURLs(root string) ([]string, error) {
	var urls []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || d.Name() != "package.json" {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}

		var payload map[string]any
		decodeErr := json.NewDecoder(file).Decode(&payload)
		closeErr := file.Close()
		if decodeErr != nil {
			return fmt.Errorf("decode %s: %w", path, decodeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", path, closeErr)
		}

		resolved, ok := payload["_resolved"].(string)
		if ok && strings.TrimSpace(resolved) != "" {
			urls = append(urls, resolved)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk node_modules: %w", err)
	}
	return uniqueSorted(urls), nil
}

func searchResolved(value any, urls *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if key == "resolved" {
				resolved, ok := item.(string)
				if ok && strings.TrimSpace(resolved) != "" {
					*urls = append(*urls, resolved)
				}
			}
			searchResolved(item, urls)
		}
	case []any:
		for _, item := range typed {
			searchResolved(item, urls)
		}
	}
}

func parseYarnResolvedURLs(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	var urls []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "resolved ") {
			continue
		}

		resolved := strings.TrimSpace(strings.TrimPrefix(line, "resolved"))
		resolved = strings.Trim(resolved, "\"'")
		if resolved != "" {
			urls = append(urls, resolved)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan yarn.lock: %w", err)
	}
	return urls, nil
}

func uniqueSorted(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))

	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}

	slices.Sort(result)
	return result
}
