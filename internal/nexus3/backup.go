package nexus3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

type DownloadRepositoryOptions struct {
	Target    Target
	Format    Format
	OutputDir string
	Workers   int
	Stdout    io.Writer
	Stderr    io.Writer
}

type DownloadMavenComponentOptions struct {
	Target     Target
	GroupID    string
	ArtifactID string
	Version    string
	OutputDir  string
	Workers    int
	Stdout     io.Writer
	Stderr     io.Writer
}

type DownloadNPMComponentOptions struct {
	Target    Target
	Name      string
	Version   string
	OutputDir string
	Workers   int
	Stdout    io.Writer
	Stderr    io.Writer
}

type UploadOptions struct {
	Target    Target
	Format    Format
	InputPath string
	Workers   int
	Stdout    io.Writer
	Stderr    io.Writer
}

type BackupManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	Format        Format            `json:"format"`
	Repository    string            `json:"repository"`
	ExportedAt    string            `json:"exportedAt"`
	Components    []BackupComponent `json:"components"`
}

type BackupComponent struct {
	Group   string        `json:"group,omitempty"`
	Name    string        `json:"name"`
	Version string        `json:"version"`
	Assets  []BackupAsset `json:"assets"`
}

type BackupAsset struct {
	RepositoryPath string            `json:"repositoryPath"`
	File           string            `json:"file"`
	ContentType    string            `json:"contentType,omitempty"`
	Checksum       map[string]string `json:"checksum,omitempty"`
	Extension      string            `json:"extension,omitempty"`
	Classifier     string            `json:"classifier,omitempty"`
}

type backupDownloadJob struct {
	DownloadURL string
	Destination string
}

type UploadFile struct {
	RepositoryPath string
	SourcePath     string
}

func DownloadRepositoryBackup(ctx context.Context, client *Client, opts DownloadRepositoryOptions) error {
	opts = normalizeDownloadRepositoryOptions(opts)

	components, err := client.ListAllComponents(ctx, opts.Target.Repository)
	if err != nil {
		return err
	}
	return downloadComponents(ctx, client, components, opts.Target.Repository, opts.Format, opts.OutputDir, opts.Workers, opts.Stdout)
}

func DownloadMavenComponent(ctx context.Context, client *Client, opts DownloadMavenComponentOptions) error {
	opts.Workers = normalizeWorkers(opts.Workers)
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}

	params := url.Values{}
	params.Set("repository", opts.Target.Repository)
	params.Set("format", "maven2")
	params.Set("maven.groupId", opts.GroupID)
	params.Set("maven.artifactId", opts.ArtifactID)
	params.Set("maven.baseVersion", opts.Version)

	components, err := client.SearchComponents(ctx, params)
	if err != nil {
		return err
	}
	components = filterMavenComponents(components, opts.GroupID, opts.ArtifactID, opts.Version)
	if len(components) == 0 {
		return fmt.Errorf("no maven component found for %s:%s:%s in repository %s", opts.GroupID, opts.ArtifactID, opts.Version, opts.Target.Repository)
	}
	return downloadComponents(ctx, client, components, opts.Target.Repository, FormatMaven, opts.OutputDir, opts.Workers, opts.Stdout)
}

func DownloadNPMComponent(ctx context.Context, client *Client, opts DownloadNPMComponentOptions) error {
	opts.Workers = normalizeWorkers(opts.Workers)
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}

	params := url.Values{}
	params.Set("repository", opts.Target.Repository)
	params.Set("format", "npm")
	params.Set("name", opts.Name)
	params.Set("version", opts.Version)

	components, err := client.SearchComponents(ctx, params)
	if err != nil {
		return err
	}
	components = filterNPMComponents(components, opts.Name, opts.Version)
	if len(components) == 0 {
		return fmt.Errorf("no npm component found for %s@%s in repository %s", opts.Name, opts.Version, opts.Target.Repository)
	}
	return downloadComponents(ctx, client, components, opts.Target.Repository, FormatNPM, opts.OutputDir, opts.Workers, opts.Stdout)
}

func UploadBackup(ctx context.Context, client *Client, opts UploadOptions) error {
	opts = normalizeUploadOptions(opts)

	switch opts.Format {
	case FormatMaven:
		return uploadMavenBackup(ctx, client, opts)
	case FormatNPM:
		return uploadNPMBackup(ctx, client, opts)
	default:
		return fmt.Errorf("unsupported format %q", opts.Format)
	}
}

func downloadComponents(ctx context.Context, client *Client, components []Component, repository string, format Format, outputDir string, workers int, stdout io.Writer) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	slices.SortFunc(components, func(a, b Component) int {
		if diff := strings.Compare(a.Group, b.Group); diff != 0 {
			return diff
		}
		if diff := strings.Compare(a.Name, b.Name); diff != 0 {
			return diff
		}
		return strings.Compare(a.Version, b.Version)
	})

	manifest := BackupManifest{
		SchemaVersion: 1,
		Format:        format,
		Repository:    repository,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		Components:    make([]BackupComponent, 0, len(components)),
	}

	var jobs []backupDownloadJob
	for _, component := range components {
		assets, err := backupAssetsForFormat(format, component)
		if err != nil {
			return err
		}
		if len(assets) == 0 {
			continue
		}

		backupComponent := BackupComponent{
			Group:   component.Group,
			Name:    component.Name,
			Version: component.Version,
			Assets:  make([]BackupAsset, 0, len(assets)),
		}

		for _, asset := range assets {
			repositoryPath, err := normalizeRepositoryPath(asset.Path)
			if err != nil {
				return err
			}

			fileRelativePath := path.Join("assets", repositoryPath)
			backupComponent.Assets = append(backupComponent.Assets, BackupAsset{
				RepositoryPath: repositoryPath,
				File:           fileRelativePath,
				ContentType:    asset.ContentType,
				Checksum:       cloneMap(asset.Checksum),
				Extension:      asset.Maven2.Extension,
				Classifier:     asset.Maven2.Classifier,
			})
			jobs = append(jobs, backupDownloadJob{
				DownloadURL: asset.DownloadURL,
				Destination: filepath.Join(outputDir, filepath.FromSlash(fileRelativePath)),
			})
		}

		manifest.Components = append(manifest.Components, backupComponent)
	}

	if len(manifest.Components) == 0 {
		return fmt.Errorf("no %s components found in repository %s", format, repository)
	}

	fmt.Fprintf(stdout, "found %d %s components and %d assets to download\n", len(manifest.Components), format, len(jobs))
	if err := runBackupDownloads(ctx, client, jobs, workers); err != nil {
		return err
	}
	if err := writeManifest(outputDir, manifest); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "backup saved to %s\n", outputDir)
	return nil
}

func uploadMavenBackup(ctx context.Context, client *Client, opts UploadOptions) error {
	items, err := ResolveMavenUploadInputs(opts.InputPath)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no maven assets found in %s", opts.InputPath)
	}

	slices.SortFunc(items, func(a, b UploadFile) int {
		return strings.Compare(a.RepositoryPath, b.RepositoryPath)
	})
	fmt.Fprintf(opts.Stdout, "found %d maven assets to upload\n", len(items))

	errs := make(chan error, len(items))
	jobs := make(chan UploadFile)
	var wg sync.WaitGroup

	workerCount := opts.Workers
	if workerCount > len(items) {
		workerCount = len(items)
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				fmt.Fprintf(opts.Stdout, "uploading %s\n", item.RepositoryPath)
				if err := client.PutRepositoryFile(ctx, opts.Target, item.RepositoryPath, item.SourcePath); err != nil {
					errs <- err
					continue
				}
				fmt.Fprintf(opts.Stdout, "uploaded %s\n", item.RepositoryPath)
			}
		}()
	}

	for _, item := range items {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			close(errs)
			return ctx.Err()
		case jobs <- item:
		}
	}

	close(jobs)
	wg.Wait()
	close(errs)

	var uploadErrs []error
	for err := range errs {
		uploadErrs = append(uploadErrs, err)
	}
	if len(uploadErrs) > 0 {
		return fmt.Errorf("%d maven uploads failed: %w", len(uploadErrs), errors.Join(uploadErrs...))
	}

	fmt.Fprintln(opts.Stdout, "all maven uploads completed")
	return nil
}

func uploadNPMBackup(ctx context.Context, client *Client, opts UploadOptions) error {
	tarballs, err := ResolveNPMUploadInputs(opts.InputPath)
	if err != nil {
		return err
	}
	if len(tarballs) == 0 {
		return fmt.Errorf("no npm tarballs found in %s", opts.InputPath)
	}

	slices.Sort(tarballs)
	fmt.Fprintf(opts.Stdout, "found %d npm tarballs to upload\n", len(tarballs))

	errs := make(chan error, len(tarballs))
	jobs := make(chan string)
	var wg sync.WaitGroup

	workerCount := opts.Workers
	if workerCount > len(tarballs) {
		workerCount = len(tarballs)
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for tarball := range jobs {
				fmt.Fprintf(opts.Stdout, "uploading %s\n", tarball)
				if err := client.UploadNPMTarball(ctx, opts.Target, tarball); err != nil {
					errs <- err
					continue
				}
				fmt.Fprintf(opts.Stdout, "uploaded %s\n", tarball)
			}
		}()
	}

	for _, tarball := range tarballs {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			close(errs)
			return ctx.Err()
		case jobs <- tarball:
		}
	}

	close(jobs)
	wg.Wait()
	close(errs)

	var uploadErrs []error
	for err := range errs {
		uploadErrs = append(uploadErrs, err)
	}
	if len(uploadErrs) > 0 {
		return fmt.Errorf("%d npm uploads failed: %w", len(uploadErrs), errors.Join(uploadErrs...))
	}

	fmt.Fprintln(opts.Stdout, "all npm uploads completed")
	return nil
}

func backupAssetsForFormat(format Format, component Component) ([]Asset, error) {
	switch format {
	case FormatMaven:
		assets := make([]Asset, 0, len(component.Assets))
		for _, asset := range component.Assets {
			if strings.TrimSpace(asset.Path) == "" || strings.TrimSpace(asset.DownloadURL) == "" {
				continue
			}
			assets = append(assets, asset)
		}
		return assets, nil
	case FormatNPM:
		assets := make([]Asset, 0, 1)
		for _, asset := range component.Assets {
			if strings.HasSuffix(asset.Path, ".tgz") && strings.TrimSpace(asset.DownloadURL) != "" {
				assets = append(assets, asset)
			}
		}
		return assets, nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func runBackupDownloads(ctx context.Context, client *Client, jobs []backupDownloadJob, workers int) error {
	errs := make(chan error, len(jobs))
	work := make(chan backupDownloadJob)
	var wg sync.WaitGroup

	workerCount := workers
	if workerCount > len(jobs) {
		workerCount = len(jobs)
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range work {
				if err := client.DownloadAsset(ctx, job.DownloadURL, job.Destination); err != nil {
					errs <- err
				}
			}
		}()
	}

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			close(work)
			wg.Wait()
			close(errs)
			return ctx.Err()
		case work <- job:
		}
	}

	close(work)
	wg.Wait()
	close(errs)

	var downloadErrs []error
	for err := range errs {
		downloadErrs = append(downloadErrs, err)
	}
	if len(downloadErrs) > 0 {
		return fmt.Errorf("%d asset downloads failed: %w", len(downloadErrs), errors.Join(downloadErrs...))
	}
	return nil
}

func writeManifest(outputDir string, manifest BackupManifest) error {
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal backup manifest: %w", err)
	}

	manifestPath := filepath.Join(outputDir, ManifestFilename)
	if err := os.WriteFile(manifestPath, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write backup manifest: %w", err)
	}
	return nil
}

func ResolveMavenUploadInputs(inputPath string) ([]UploadFile, error) {
	manifest, manifestRoot, found, err := loadManifest(inputPath)
	if err != nil {
		return nil, err
	}
	if found {
		if manifest.Format != FormatMaven {
			return nil, fmt.Errorf("backup manifest format is %s, expected %s", manifest.Format, FormatMaven)
		}
		items := make([]UploadFile, 0)
		for _, component := range manifest.Components {
			for _, asset := range component.Assets {
				sourcePath, err := resolveManifestFile(manifestRoot, asset.File)
				if err != nil {
					return nil, err
				}
				items = append(items, UploadFile{
					RepositoryPath: asset.RepositoryPath,
					SourcePath:     sourcePath,
				})
			}
		}
		return items, nil
	}

	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("stat input path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("maven upload without a manifest requires a directory: %s", inputPath)
	}

	items := make([]UploadFile, 0)
	err = filepath.WalkDir(inputPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base == ManifestFilename || base == LegacyManifestFilename || isChecksumFile(path) {
			return nil
		}

		relativePath, err := filepath.Rel(inputPath, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}
		items = append(items, UploadFile{
			RepositoryPath: filepath.ToSlash(relativePath),
			SourcePath:     path,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan maven upload input: %w", err)
	}
	return items, nil
}

func ResolveNPMUploadInputs(inputPath string) ([]string, error) {
	manifest, manifestRoot, found, err := loadManifest(inputPath)
	if err != nil {
		return nil, err
	}
	if found {
		if manifest.Format != FormatNPM {
			return nil, fmt.Errorf("backup manifest format is %s, expected %s", manifest.Format, FormatNPM)
		}

		tarballs := make([]string, 0)
		for _, component := range manifest.Components {
			for _, asset := range component.Assets {
				if !strings.HasSuffix(asset.RepositoryPath, ".tgz") {
					continue
				}
				sourcePath, err := resolveManifestFile(manifestRoot, asset.File)
				if err != nil {
					return nil, err
				}
				tarballs = append(tarballs, sourcePath)
			}
		}
		return tarballs, nil
	}

	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("stat input path: %w", err)
	}
	if !info.IsDir() {
		if strings.HasSuffix(inputPath, ".tgz") {
			return []string{inputPath}, nil
		}
		return nil, fmt.Errorf("npm upload requires a .tgz file or a directory: %s", inputPath)
	}

	tarballs := make([]string, 0)
	err = filepath.WalkDir(inputPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".tgz") {
			tarballs = append(tarballs, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan npm upload input: %w", err)
	}
	return tarballs, nil
}

func loadManifest(inputPath string) (BackupManifest, string, bool, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return BackupManifest{}, "", false, fmt.Errorf("stat input path: %w", err)
	}

	var manifestPath string
	var root string
	switch {
	case info.IsDir():
		root = inputPath
	case filepath.Base(inputPath) == ManifestFilename, filepath.Base(inputPath) == LegacyManifestFilename:
		manifestPath = inputPath
		root = filepath.Dir(inputPath)
	default:
		return BackupManifest{}, "", false, nil
	}

	if info.IsDir() {
		manifestPath = filepath.Join(inputPath, ManifestFilename)
		if _, err := os.Stat(manifestPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				manifestPath = filepath.Join(inputPath, LegacyManifestFilename)
			} else {
				return BackupManifest{}, "", false, fmt.Errorf("stat manifest: %w", err)
			}
		}
	}

	if _, err := os.Stat(manifestPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BackupManifest{}, "", false, nil
		}
		return BackupManifest{}, "", false, fmt.Errorf("stat manifest: %w", err)
	}

	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return BackupManifest{}, "", false, fmt.Errorf("read manifest: %w", err)
	}

	var manifest BackupManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return BackupManifest{}, "", false, fmt.Errorf("decode manifest: %w", err)
	}
	return manifest, root, true, nil
}

func resolveManifestFile(root, relativePath string) (string, error) {
	cleanRelativePath := filepath.Clean(filepath.FromSlash(relativePath))
	if cleanRelativePath == "." || cleanRelativePath == "" {
		return "", fmt.Errorf("manifest file path is empty")
	}
	if strings.HasPrefix(cleanRelativePath, ".."+string(os.PathSeparator)) || cleanRelativePath == ".." {
		return "", fmt.Errorf("manifest file path escapes root: %s", relativePath)
	}

	absolutePath := filepath.Join(root, cleanRelativePath)
	if _, err := os.Stat(absolutePath); err != nil {
		return "", fmt.Errorf("manifest asset file %s: %w", absolutePath, err)
	}
	return absolutePath, nil
}

func filterMavenComponents(components []Component, groupID, artifactID, version string) []Component {
	filtered := make([]Component, 0, len(components))
	for _, component := range components {
		if component.Group == groupID && component.Name == artifactID && component.Version == version {
			filtered = append(filtered, component)
		}
	}
	return filtered
}

func filterNPMComponents(components []Component, name, version string) []Component {
	filtered := make([]Component, 0, len(components))
	for _, component := range components {
		if component.Name == name && component.Version == version {
			filtered = append(filtered, component)
		}
	}
	return filtered
}

func normalizeDownloadRepositoryOptions(opts DownloadRepositoryOptions) DownloadRepositoryOptions {
	opts.Workers = normalizeWorkers(opts.Workers)
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	return opts
}

func normalizeUploadOptions(opts UploadOptions) UploadOptions {
	opts.Workers = normalizeWorkers(opts.Workers)
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	return opts
}

func normalizeWorkers(workers int) int {
	if workers > 0 {
		return workers
	}
	if runtime.NumCPU() > 0 {
		return runtime.NumCPU()
	}
	return 1
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func isChecksumFile(filename string) bool {
	switch filepath.Ext(filename) {
	case ".md5", ".sha1", ".sha256", ".sha512":
		return true
	default:
		return false
	}
}
