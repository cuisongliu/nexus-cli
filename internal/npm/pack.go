package npm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type PackOptions struct {
	Stdout io.Writer
	Stderr io.Writer
}

type packSpec struct {
	Dir         string
	NameVersion string
}

func PackTarballs(ctx context.Context, projectPath, storePath string, opts PackOptions) error {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}

	if err := os.MkdirAll(storePath, 0o755); err != nil {
		return fmt.Errorf("create store path: %w", err)
	}

	root, err := detectPackRoot(projectPath)
	if err != nil {
		return err
	}

	specs, err := collectPackSpecs(root)
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		fmt.Fprintln(opts.Stdout, "no packages found to pack")
		return nil
	}

	slices.SortFunc(specs, func(a, b packSpec) int {
		return strings.Compare(a.NameVersion, b.NameVersion)
	})
	fmt.Fprintf(opts.Stdout, "found %d packages to pack\n", len(specs))

	var packErrs []error
	for _, spec := range specs {
		fmt.Fprintf(opts.Stdout, "packing %s\n", spec.NameVersion)
		output, err := runNpmPack(ctx, spec.Dir, storePath)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "pack failed %s: %v\n", spec.NameVersion, err)
			packErrs = append(packErrs, fmt.Errorf("%s: %w", spec.NameVersion, err))
			continue
		}
		if strings.TrimSpace(output) != "" {
			fmt.Fprint(opts.Stdout, output)
			if !strings.HasSuffix(output, "\n") {
				fmt.Fprintln(opts.Stdout)
			}
		}
	}

	if len(packErrs) > 0 {
		return fmt.Errorf("%d packages failed to pack: %w", len(packErrs), errors.Join(packErrs...))
	}

	fmt.Fprintln(opts.Stdout, "all packs completed")
	return nil
}

func detectPackRoot(projectPath string) (string, error) {
	pnpmRoot := filepath.Join(projectPath, ".pnpm")
	if info, err := os.Stat(pnpmRoot); err == nil && info.IsDir() {
		return pnpmRoot, nil
	}

	nodeModulesRoot := filepath.Join(projectPath, "node_modules")
	if info, err := os.Stat(nodeModulesRoot); err == nil && info.IsDir() {
		return nodeModulesRoot, nil
	}

	return "", fmt.Errorf("neither %s nor %s exists", pnpmRoot, nodeModulesRoot)
}

func collectPackSpecs(root string) ([]packSpec, error) {
	seen := make(map[string]struct{})
	specs := make([]packSpec, 0)

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

		var payload struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		decodeErr := json.NewDecoder(file).Decode(&payload)
		closeErr := file.Close()
		if decodeErr != nil {
			return fmt.Errorf("decode %s: %w", path, decodeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", path, closeErr)
		}
		if payload.Name == "" || payload.Version == "" {
			return nil
		}

		nameVersion := payload.Name + "@" + payload.Version
		if _, ok := seen[nameVersion]; ok {
			return nil
		}
		seen[nameVersion] = struct{}{}
		specs = append(specs, packSpec{
			Dir:         filepath.Dir(path),
			NameVersion: nameVersion,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk pack root: %w", err)
	}
	return specs, nil
}

func runNpmPack(ctx context.Context, packDir, storePath string) (string, error) {
	cmd := exec.CommandContext(ctx, "npm", "pack", "--pack-destination", storePath)
	cmd.Dir = packDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
