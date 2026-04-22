package nexus3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"nexus3/internal/httpx"
)

type Format string

const (
	FormatMaven Format = "maven"
	FormatNPM   Format = "npm"
)

const ManifestFilename = "nexus3-backup.json"

type Target struct {
	BaseURL    string
	Repository string
}

type Client struct {
	BaseURL    string
	Username   string
	Password   string
	HTTPClient *http.Client
}

type Component struct {
	ID         string  `json:"id"`
	Repository string  `json:"repository"`
	Format     string  `json:"format"`
	Group      string  `json:"group"`
	Name       string  `json:"name"`
	Version    string  `json:"version"`
	Assets     []Asset `json:"assets"`
}

type Asset struct {
	DownloadURL string            `json:"downloadUrl"`
	Path        string            `json:"path"`
	ID          string            `json:"id"`
	Repository  string            `json:"repository"`
	Format      string            `json:"format"`
	Checksum    map[string]string `json:"checksum"`
	ContentType string            `json:"contentType"`
	Maven2      MavenAsset        `json:"maven2"`
}

type MavenAsset struct {
	GroupID    string `json:"groupId"`
	ArtifactID string `json:"artifactId"`
	Version    string `json:"version"`
	Extension  string `json:"extension"`
	Classifier string `json:"classifier"`
}

type componentPage struct {
	Items             []Component `json:"items"`
	ContinuationToken string      `json:"continuationToken"`
}

func ParseFormat(raw string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(FormatMaven), "maven2":
		return FormatMaven, nil
	case string(FormatNPM):
		return FormatNPM, nil
	default:
		return "", fmt.Errorf("unsupported format %q", raw)
	}
}

func ResolveTarget(baseURL, repository, repoURL string) (Target, error) {
	if strings.TrimSpace(repoURL) != "" {
		target, err := ParseRepositoryURL(repoURL)
		if err != nil {
			return Target{}, err
		}
		return target, nil
	}

	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(repository) == "" {
		return Target{}, errors.New("either --repo-url or both --base-url and --repository are required")
	}

	normalizedBaseURL, err := httpx.NormalizeBaseURL(baseURL)
	if err != nil {
		return Target{}, fmt.Errorf("invalid base url: %w", err)
	}

	return Target{
		BaseURL:    normalizedBaseURL,
		Repository: strings.TrimSpace(repository),
	}, nil
}

func ParseRepositoryURL(raw string) (Target, error) {
	normalizedURL, err := httpx.NormalizeBaseURL(raw)
	if err != nil {
		return Target{}, fmt.Errorf("invalid repository url: %w", err)
	}

	parsed, err := url.Parse(normalizedURL)
	if err != nil {
		return Target{}, fmt.Errorf("parse repository url: %w", err)
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	repositoryIndex := -1
	for i, segment := range segments {
		if segment == "repository" {
			repositoryIndex = i
			break
		}
	}
	if repositoryIndex < 0 || repositoryIndex != len(segments)-2 {
		return Target{}, fmt.Errorf("repository url must end with /repository/<name>: %s", raw)
	}

	repository := segments[repositoryIndex+1]
	baseSegments := segments[:repositoryIndex]
	if len(baseSegments) == 0 {
		parsed.Path = ""
	} else {
		parsed.Path = "/" + strings.Join(baseSegments, "/")
	}
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	baseURL := strings.TrimRight(parsed.String(), "/")
	return Target{
		BaseURL:    baseURL,
		Repository: repository,
	}, nil
}

func NewClient(baseURL, username, password string, httpClient *http.Client) (*Client, error) {
	normalizedBaseURL, err := httpx.NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		BaseURL:    normalizedBaseURL,
		Username:   username,
		Password:   password,
		HTTPClient: httpClient,
	}, nil
}

func (t Target) RepositoryURL() string {
	return strings.TrimRight(t.BaseURL, "/") + "/repository/" + url.PathEscape(t.Repository)
}

func (c *Client) ListAllComponents(ctx context.Context, repository string) ([]Component, error) {
	params := url.Values{}
	params.Set("repository", repository)
	return c.fetchComponentPages(ctx, "/components", params)
}

func (c *Client) SearchComponents(ctx context.Context, params url.Values) ([]Component, error) {
	return c.fetchComponentPages(ctx, "/search", params)
}

func (c *Client) DownloadAsset(ctx context.Context, downloadURL, destination string) error {
	requestURL, err := httpx.NormalizeRequestURL(downloadURL)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(pathDirFS(destination), 0o755); err != nil {
		return fmt.Errorf("create asset directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("create asset request: %w", err)
	}
	req.SetBasicAuth(c.Username, c.Password)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download asset %s: %w", requestURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("download asset %s: unexpected status %s: %s", requestURL, resp.Status, strings.TrimSpace(string(body)))
	}

	tempFile := destination + ".part"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("create asset file %s: %w", tempFile, err)
	}

	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(tempFile)
		return fmt.Errorf("write asset file %s: %w", tempFile, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempFile)
		return fmt.Errorf("close asset file %s: %w", tempFile, err)
	}
	if err := os.Rename(tempFile, destination); err != nil {
		_ = os.Remove(tempFile)
		return fmt.Errorf("rename asset file %s: %w", destination, err)
	}
	return nil
}

func (c *Client) PutRepositoryFile(ctx context.Context, target Target, repositoryPath, sourcePath string) error {
	normalizedPath, err := normalizeRepositoryPath(repositoryPath)
	if err != nil {
		return err
	}

	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", sourcePath, err)
	}
	defer file.Close()

	requestURL := target.RepositoryURL() + "/" + normalizedPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, requestURL, file)
	if err != nil {
		return fmt.Errorf("create put request for %s: %w", sourcePath, err)
	}
	req.SetBasicAuth(c.Username, c.Password)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload %s: %w", sourcePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("upload %s: unexpected status %s: %s", sourcePath, resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) UploadNPMTarball(ctx context.Context, target Target, tarballPath string) error {
	file, err := os.Open(tarballPath)
	if err != nil {
		return fmt.Errorf("open npm tarball %s: %w", tarballPath, err)
	}
	defer file.Close()

	bodyReader, bodyWriter := io.Pipe()
	formWriter := multipart.NewWriter(bodyWriter)

	go func() {
		defer bodyWriter.Close()
		defer formWriter.Close()

		part, err := formWriter.CreateFormFile("npm.asset", path.Base(tarballPath))
		if err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = bodyWriter.CloseWithError(err)
		}
	}()

	params := url.Values{}
	params.Set("repository", target.Repository)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL("/components")+"?"+params.Encode(), bodyReader)
	if err != nil {
		return fmt.Errorf("create npm upload request: %w", err)
	}
	req.Header.Set("Content-Type", formWriter.FormDataContentType())
	req.SetBasicAuth(c.Username, c.Password)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload npm tarball %s: %w", tarballPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("upload npm tarball %s: unexpected status %s: %s", tarballPath, resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) fetchComponentPages(ctx context.Context, endpoint string, params url.Values) ([]Component, error) {
	query := cloneValues(params)
	var items []Component

	for {
		var page componentPage
		if err := c.getJSON(ctx, endpoint, query, &page); err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if page.ContinuationToken == "" {
			break
		}
		query.Set("continuationToken", page.ContinuationToken)
	}
	return items, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, params url.Values, out any) error {
	requestURL := c.apiURL(endpoint)
	if encoded := params.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("create request %s: %w", requestURL, err)
	}
	req.SetBasicAuth(c.Username, c.Password)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", requestURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("request %s: unexpected status %s: %s", requestURL, resp.Status, strings.TrimSpace(string(body)))
	}

	if err := jsonNewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", requestURL, err)
	}
	return nil
}

func (c *Client) apiURL(endpoint string) string {
	return strings.TrimRight(c.BaseURL, "/") + "/service/rest/v1" + endpoint
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

func normalizeRepositoryPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "/")
	trimmed = path.Clean(trimmed)
	if trimmed == "." || trimmed == "" {
		return "", fmt.Errorf("repository path is empty")
	}
	if strings.HasPrefix(trimmed, "../") || trimmed == ".." {
		return "", fmt.Errorf("repository path escapes repository root: %s", raw)
	}
	return trimmed, nil
}

func pathDirFS(filename string) string {
	index := strings.LastIndex(filename, string(os.PathSeparator))
	if index < 0 {
		return "."
	}
	if index == 0 {
		return string(os.PathSeparator)
	}
	return filename[:index]
}
