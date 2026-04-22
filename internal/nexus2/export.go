package nexus2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	nx3 "nexus3/internal/nexus3"
)

type DownloadRepositoryOptions struct {
	Target    Target
	Format    nx3.Format
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

type discoveredAsset struct {
	DownloadURL    string
	RepositoryPath string
}

type npmVersionDocument struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Dist    struct {
		Tarball string `json:"tarball"`
	} `json:"dist"`
}

type npmMetadataDocument struct {
	Name     string                        `json:"name"`
	Versions map[string]npmVersionDocument `json:"versions"`
}

var hrefPattern = regexp.MustCompile(`href="([^"]+)"`)

func DownloadRepositoryBackup(ctx context.Context, client *Client, opts DownloadRepositoryOptions) error {
	opts = normalizeDownloadOptions(opts)

	switch opts.Format {
	case nx3.FormatMaven:
		assets, err := crawlMavenAssets(ctx, client, opts.Target)
		if err != nil {
			return err
		}
		return writeBackupFromAssets(ctx, client, opts.OutputDir, opts.Target.Repository, opts.Format, buildMavenComponents(assets), opts.Workers, opts.Stdout)
	case nx3.FormatNPM:
		components, err := crawlNPMComponents(ctx, client, opts.Target)
		if err != nil {
			return err
		}
		return writeBackupFromAssets(ctx, client, opts.OutputDir, opts.Target.Repository, opts.Format, components, opts.Workers, opts.Stdout)
	default:
		return fmt.Errorf("unsupported format %q", opts.Format)
	}
}

func DownloadMavenComponent(ctx context.Context, client *Client, opts DownloadMavenComponentOptions) error {
	opts.Workers = normalizeWorkers(opts.Workers)
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}

	repoURL := strings.TrimRight(opts.Target.RepositoryURL(), "/") + "/" + strings.ReplaceAll(opts.GroupID, ".", "/") + "/" + opts.ArtifactID + "/" + opts.Version + "/"
	assets, err := crawlHTMLAssets(ctx, client, opts.Target, repoURL)
	if err != nil {
		return err
	}
	if len(assets) == 0 {
		return fmt.Errorf("no maven component found for %s:%s:%s in repository %s", opts.GroupID, opts.ArtifactID, opts.Version, opts.Target.Repository)
	}

	component := nx3.BackupComponent{
		Group:   opts.GroupID,
		Name:    opts.ArtifactID,
		Version: opts.Version,
		Assets:  make([]nx3.BackupAsset, 0, len(assets)),
	}
	for _, asset := range assets {
		component.Assets = append(component.Assets, toBackupAsset(asset))
	}
	return writeBackupFromAssets(ctx, client, opts.OutputDir, opts.Target.Repository, nx3.FormatMaven, []nx3.BackupComponent{component}, opts.Workers, opts.Stdout)
}

func DownloadNPMComponent(ctx context.Context, client *Client, opts DownloadNPMComponentOptions) error {
	opts.Workers = normalizeWorkers(opts.Workers)
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}

	component, err := fetchNPMComponent(ctx, client, opts.Target, opts.Name, opts.Version)
	if err != nil {
		return err
	}
	return writeBackupFromAssets(ctx, client, opts.OutputDir, opts.Target.Repository, nx3.FormatNPM, []nx3.BackupComponent{component}, opts.Workers, opts.Stdout)
}

func crawlMavenAssets(ctx context.Context, client *Client, target Target) ([]discoveredAsset, error) {
	return crawlHTMLAssets(ctx, client, target, ensureTrailingSlash(target.RepositoryURL()))
}

func crawlHTMLAssets(ctx context.Context, client *Client, target Target, startURL string) ([]discoveredAsset, error) {
	rootURL := ensureTrailingSlash(target.RepositoryURL())
	rootParsed, err := url.Parse(rootURL)
	if err != nil {
		return nil, fmt.Errorf("parse repository url: %w", err)
	}

	queue := []string{ensureTrailingSlash(startURL)}
	visited := map[string]struct{}{}
	var assets []discoveredAsset

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}

		body, contentType, err := getContent(ctx, client, current)
		if err != nil {
			return nil, err
		}
		if !strings.Contains(strings.ToLower(contentType), "html") && !strings.Contains(string(body), "<html") {
			continue
		}

		for _, href := range extractLinks(string(body)) {
			if href == "../" || href == ".." || href == "" {
				continue
			}
			resolved, err := resolveRepositoryURL(current, href)
			if err != nil || !strings.HasPrefix(resolved, rootURL) {
				continue
			}
			relativePath, err := repositoryRelativePath(rootParsed, resolved)
			if err != nil {
				continue
			}

			if strings.HasSuffix(href, "/") {
				queue = append(queue, ensureTrailingSlash(resolved))
				continue
			}
			if isChecksumFile(relativePath) {
				continue
			}
			assets = append(assets, discoveredAsset{
				DownloadURL:    resolved,
				RepositoryPath: relativePath,
			})
		}
	}

	slices.SortFunc(assets, func(a, b discoveredAsset) int {
		return strings.Compare(a.RepositoryPath, b.RepositoryPath)
	})
	return assets, nil
}

func crawlNPMComponents(ctx context.Context, client *Client, target Target) ([]nx3.BackupComponent, error) {
	rootURL := ensureTrailingSlash(target.RepositoryURL())
	queue := []string{rootURL}
	visited := map[string]struct{}{}
	components := make([]nx3.BackupComponent, 0)
	seenVersions := make(map[string]struct{})

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}

		body, contentType, err := getContent(ctx, client, current)
		if err != nil {
			return nil, err
		}

		lowerContentType := strings.ToLower(contentType)
		switch {
		case strings.Contains(lowerContentType, "application/json") || strings.HasPrefix(strings.TrimSpace(string(body)), "{"):
			componentSet, err := parseNPMMetadata(target, body)
			if err != nil {
				return nil, err
			}
			for _, component := range componentSet {
				key := component.Name + "@" + component.Version
				if _, ok := seenVersions[key]; ok {
					continue
				}
				seenVersions[key] = struct{}{}
				components = append(components, component)
			}
		case strings.Contains(lowerContentType, "html") || strings.Contains(string(body), "<html"):
			for _, href := range extractLinks(string(body)) {
				if href == "../" || href == ".." || href == "" {
					continue
				}
				resolved, err := resolveRepositoryURL(current, href)
				if err != nil || !strings.HasPrefix(resolved, rootURL) {
					continue
				}
				if strings.HasSuffix(href, ".tgz") || strings.HasSuffix(href, ".tar") {
					continue
				}
				queue = append(queue, resolved)
			}
		}
	}

	slices.SortFunc(components, func(a, b nx3.BackupComponent) int {
		if diff := strings.Compare(a.Name, b.Name); diff != 0 {
			return diff
		}
		return strings.Compare(a.Version, b.Version)
	})
	return components, nil
}

func fetchNPMComponent(ctx context.Context, client *Client, target Target, packageName, version string) (nx3.BackupComponent, error) {
	metadataURL := strings.TrimRight(target.RepositoryURL(), "/") + "/" + url.PathEscape(packageName)
	body, _, err := getContent(ctx, client, metadataURL)
	if err != nil {
		return nx3.BackupComponent{}, err
	}

	components, err := parseNPMMetadata(target, body)
	if err != nil {
		return nx3.BackupComponent{}, err
	}
	for _, component := range components {
		if component.Name == packageName && component.Version == version {
			return component, nil
		}
	}
	return nx3.BackupComponent{}, fmt.Errorf("no npm component found for %s@%s in repository %s", packageName, version, target.Repository)
}

func parseNPMMetadata(target Target, body []byte) ([]nx3.BackupComponent, error) {
	var metadata npmMetadataDocument
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, fmt.Errorf("decode npm metadata: %w", err)
	}

	components := make([]nx3.BackupComponent, 0, len(metadata.Versions))
	for _, versionDoc := range metadata.Versions {
		if versionDoc.Name == "" || versionDoc.Version == "" || versionDoc.Dist.Tarball == "" {
			continue
		}
		repositoryPath, err := repositoryPathFromDownloadURL(target, versionDoc.Dist.Tarball)
		if err != nil {
			return nil, err
		}
		components = append(components, nx3.BackupComponent{
			Name:    versionDoc.Name,
			Version: versionDoc.Version,
			Assets: []nx3.BackupAsset{
				{
					RepositoryPath: repositoryPath,
					File:           path.Join("assets", repositoryPath),
				},
			},
		})
	}
	return components, nil
}

func buildMavenComponents(assets []discoveredAsset) []nx3.BackupComponent {
	grouped := make(map[string]*nx3.BackupComponent)
	order := make([]string, 0)

	for _, asset := range assets {
		groupID, artifactID, version := inferMavenCoordinates(asset.RepositoryPath)
		key := groupID + "|" + artifactID + "|" + version
		component, ok := grouped[key]
		if !ok {
			component = &nx3.BackupComponent{
				Group:   groupID,
				Name:    artifactID,
				Version: version,
			}
			grouped[key] = component
			order = append(order, key)
		}
		component.Assets = append(component.Assets, toBackupAsset(asset))
	}

	components := make([]nx3.BackupComponent, 0, len(order))
	for _, key := range order {
		components = append(components, *grouped[key])
	}
	return components
}

func inferMavenCoordinates(repositoryPath string) (string, string, string) {
	parts := strings.Split(strings.Trim(repositoryPath, "/"), "/")
	if len(parts) < 4 {
		return "", repositoryPath, ""
	}
	groupParts := parts[:len(parts)-3]
	artifactID := parts[len(parts)-3]
	version := parts[len(parts)-2]
	return strings.Join(groupParts, "."), artifactID, version
}

func writeBackupFromAssets(ctx context.Context, client *Client, outputDir, repository string, format nx3.Format, components []nx3.BackupComponent, workers int, stdout io.Writer) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if len(components) == 0 {
		return fmt.Errorf("no %s components found in repository %s", format, repository)
	}

	manifest := nx3.BackupManifest{
		SchemaVersion: 1,
		Format:        format,
		Repository:    repository,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		Components:    components,
	}

	for i := range manifest.Components {
		for j := range manifest.Components[i].Assets {
			fileRelativePath := manifest.Components[i].Assets[j].File
			if fileRelativePath == "" {
				fileRelativePath = path.Join("assets", manifest.Components[i].Assets[j].RepositoryPath)
				manifest.Components[i].Assets[j].File = fileRelativePath
			}
		}
	}

	jobItems := make([]downloadJob, 0, len(manifest.Components))
	for _, component := range manifest.Components {
		for _, asset := range component.Assets {
			jobItems = append(jobItems, downloadJob{
				DownloadURL: assetDownloadURL(client.BaseURL, repository, asset.RepositoryPath),
				Destination: filepath.Join(outputDir, filepath.FromSlash(asset.File)),
			})
		}
	}

	fmt.Fprintf(stdout, "found %d %s components and %d assets to download\n", len(manifest.Components), format, len(jobItems))
	if err := runDownloads(ctx, client, jobItems, workers); err != nil {
		return err
	}
	if err := writeManifest(outputDir, manifest); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "backup saved to %s\n", outputDir)
	return nil
}

type downloadJob struct {
	DownloadURL string
	Destination string
}

func runDownloads(ctx context.Context, client *Client, jobs []downloadJob, workers int) error {
	errs := make(chan error, len(jobs))
	work := make(chan downloadJob)
	var wg sync.WaitGroup

	workerCount := workers
	if workerCount > len(jobs) {
		workerCount = len(jobs)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range work {
				if err := downloadFile(ctx, client, job.DownloadURL, job.Destination); err != nil {
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

func downloadFile(ctx context.Context, client *Client, requestURL, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("create request %s: %w", requestURL, err)
	}
	req.SetBasicAuth(client.Username, client.Password)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", requestURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("download %s: unexpected status %s: %s", requestURL, resp.Status, strings.TrimSpace(string(body)))
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}

	tempFile := destination + ".part"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("create %s: %w", tempFile, err)
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(tempFile)
		return fmt.Errorf("write %s: %w", tempFile, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempFile)
		return fmt.Errorf("close %s: %w", tempFile, err)
	}
	if err := os.Rename(tempFile, destination); err != nil {
		_ = os.Remove(tempFile)
		return fmt.Errorf("rename %s: %w", destination, err)
	}
	return nil
}

func getContent(ctx context.Context, client *Client, requestURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request %s: %w", requestURL, err)
	}
	req.SetBasicAuth(client.Username, client.Password)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request %s: %w", requestURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return nil, "", fmt.Errorf("request %s: unexpected status %s: %s", requestURL, resp.Status, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", requestURL, err)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

func extractLinks(html string) []string {
	matches := hrefPattern.FindAllStringSubmatch(html, -1)
	links := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		links = append(links, match[1])
	}
	return links
}

func resolveRepositoryURL(baseURL, href string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	link, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(link).String(), nil
}

func repositoryRelativePath(root *url.URL, assetURL string) (string, error) {
	parsed, err := url.Parse(assetURL)
	if err != nil {
		return "", err
	}
	relativePath := strings.TrimPrefix(parsed.Path, ensureTrailingSlash(root.Path))
	relativePath = strings.TrimPrefix(relativePath, "/")
	return relativePath, nil
}

func repositoryPathFromDownloadURL(target Target, downloadURL string) (string, error) {
	rootParsed, err := url.Parse(ensureTrailingSlash(target.RepositoryURL()))
	if err != nil {
		return "", err
	}
	return repositoryRelativePath(rootParsed, downloadURL)
}

func toBackupAsset(asset discoveredAsset) nx3.BackupAsset {
	return nx3.BackupAsset{
		RepositoryPath: asset.RepositoryPath,
		File:           path.Join("assets", asset.RepositoryPath),
	}
}

func ensureTrailingSlash(value string) string {
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}

func writeManifest(outputDir string, manifest nx3.BackupManifest) error {
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal backup manifest: %w", err)
	}
	return os.WriteFile(filepath.Join(outputDir, nx3.ManifestFilename), append(body, '\n'), 0o644)
}

func assetDownloadURL(baseURL, repository, repositoryPath string) string {
	return strings.TrimRight(baseURL, "/") + "/content/repositories/" + repository + "/" + repositoryPath
}

func normalizeDownloadOptions(opts DownloadRepositoryOptions) DownloadRepositoryOptions {
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
	if workers < 1 {
		return 1
	}
	return workers
}

func isChecksumFile(filename string) bool {
	switch filepath.Ext(filename) {
	case ".md5", ".sha1", ".sha256", ".sha512":
		return true
	default:
		return false
	}
}
