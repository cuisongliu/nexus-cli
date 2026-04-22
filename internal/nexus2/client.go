package nexus2

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"nexus-cli/internal/httpx"
)

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

type npmPackageMetadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Main        string `json:"main,omitempty"`
}

func ResolveTarget(baseURL, repository, repoURL string) (Target, error) {
	if strings.TrimSpace(repoURL) != "" {
		return ParseRepositoryURL(repoURL)
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(repository) == "" {
		return Target{}, fmt.Errorf("either --repo-url or both --base-url and --repository are required")
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
	contentIndex := -1
	for i := 0; i+1 < len(segments); i++ {
		if segments[i] == "content" && segments[i+1] == "repositories" {
			contentIndex = i
			break
		}
	}
	if contentIndex < 0 || contentIndex != len(segments)-3 {
		return Target{}, fmt.Errorf("repository url must end with /content/repositories/<name>: %s", raw)
	}

	repository := segments[contentIndex+2]
	baseSegments := segments[:contentIndex]
	if len(baseSegments) == 0 {
		parsed.Path = ""
	} else {
		parsed.Path = "/" + strings.Join(baseSegments, "/")
	}
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return Target{
		BaseURL:    strings.TrimRight(parsed.String(), "/"),
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
	return strings.TrimRight(t.BaseURL, "/") + "/content/repositories/" + url.PathEscape(t.Repository)
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

func (c *Client) PublishNPMTarball(ctx context.Context, target Target, tarballPath string) error {
	metadata, tarballBytes, err := readNPMPackageFromTarball(tarballPath)
	if err != nil {
		return err
	}

	filename := path.Base(tarballPath)
	attachmentData := base64.StdEncoding.EncodeToString(tarballBytes)
	shasum := fmt.Sprintf("%x", sha1.Sum(tarballBytes))

	payload := map[string]any{
		"_id":  metadata.Name,
		"name": metadata.Name,
		"dist-tags": map[string]string{
			"latest": metadata.Version,
		},
		"versions": map[string]any{
			metadata.Version: map[string]any{
				"_id":         metadata.Name + "@" + metadata.Version,
				"name":        metadata.Name,
				"version":     metadata.Version,
				"description": metadata.Description,
				"main":        metadata.Main,
				"dist": map[string]string{
					"shasum":  shasum,
					"tarball": target.RepositoryURL() + "/" + npmPackageTarballPath(metadata.Name, filename),
				},
			},
		},
		"_attachments": map[string]any{
			filename: map[string]any{
				"content_type": "application/octet-stream",
				"data":         attachmentData,
				"length":       len(tarballBytes),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal npm publish payload for %s: %w", tarballPath, err)
	}

	requestURL := target.RepositoryURL() + "/" + url.PathEscape(metadata.Name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, requestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create npm publish request for %s: %w", tarballPath, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.Username, c.Password)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("publish npm tarball %s: %w", tarballPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("publish npm tarball %s: unexpected status %s: %s", tarballPath, resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func readNPMPackageFromTarball(tarballPath string) (npmPackageMetadata, []byte, error) {
	tarballBytes, err := os.ReadFile(tarballPath)
	if err != nil {
		return npmPackageMetadata{}, nil, fmt.Errorf("read npm tarball %s: %w", tarballPath, err)
	}

	gzipReader, err := gzip.NewReader(bytes.NewReader(tarballBytes))
	if err != nil {
		return npmPackageMetadata{}, nil, fmt.Errorf("open gzip tarball %s: %w", tarballPath, err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return npmPackageMetadata{}, nil, fmt.Errorf("read tarball %s: %w", tarballPath, err)
		}
		if path.Base(header.Name) != "package.json" {
			continue
		}

		var metadata npmPackageMetadata
		if err := json.NewDecoder(tarReader).Decode(&metadata); err != nil {
			return npmPackageMetadata{}, nil, fmt.Errorf("decode package.json from %s: %w", tarballPath, err)
		}
		if metadata.Name == "" || metadata.Version == "" {
			return npmPackageMetadata{}, nil, fmt.Errorf("package.json in %s is missing name or version", tarballPath)
		}
		return metadata, tarballBytes, nil
	}

	return npmPackageMetadata{}, nil, fmt.Errorf("package.json not found in %s", tarballPath)
}

func npmPackageTarballPath(packageName, filename string) string {
	return npmPackageMetadataPath(packageName) + "/-/" + url.PathEscape(filename)
}

func npmPackageMetadataPath(packageName string) string {
	parts := strings.Split(packageName, "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(part))
	}
	return strings.Join(escaped, "/")
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
