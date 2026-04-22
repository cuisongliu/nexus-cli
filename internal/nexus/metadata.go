package nexus

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type mavenMetadata struct {
	Versioning struct {
		Versions []string `xml:"versions>version"`
	} `xml:"versioning"`
}

type versionNumber struct {
	Raw   string
	Major int
	Minor int
	Patch int
}

var trimSuffixPattern = regexp.MustCompile(`-.*$`)

func LatestVersion(ctx context.Context, baseURL, repository, groupID, artifactID string, client *http.Client) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: time.Minute}
	}

	metadataURL := strings.TrimRight(baseURL, "/") + "/repository/" + strings.Trim(repository, "/") + "/" +
		path.Join(strings.ReplaceAll(groupID, ".", "/"), artifactID, "maven-metadata.xml")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("create metadata request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch metadata: unexpected status %s", resp.Status)
	}

	var metadata mavenMetadata
	if err := xml.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return "", fmt.Errorf("decode metadata: %w", err)
	}

	if len(metadata.Versioning.Versions) == 0 {
		return "", fmt.Errorf("no versions found for %s:%s", groupID, artifactID)
	}

	maxVersion, err := maxVersion(metadata.Versioning.Versions)
	if err != nil {
		return "", err
	}
	return maxVersion.Raw, nil
}

func maxVersion(versions []string) (versionNumber, error) {
	current := versionNumber{Raw: "0.0.0"}
	for _, version := range versions {
		parsed, err := parseVersion(version)
		if err != nil {
			return versionNumber{}, err
		}
		if compareVersion(parsed, current) > 0 {
			current = parsed
		}
	}
	return current, nil
}

func parseVersion(raw string) (versionNumber, error) {
	trimmed := trimSuffixPattern.ReplaceAllString(raw, "")
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return versionNumber{}, fmt.Errorf("unsupported version format: %s", raw)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return versionNumber{}, fmt.Errorf("parse version %s: %w", raw, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return versionNumber{}, fmt.Errorf("parse version %s: %w", raw, err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return versionNumber{}, fmt.Errorf("parse version %s: %w", raw, err)
	}

	return versionNumber{
		Raw:   raw,
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

func compareVersion(a, b versionNumber) int {
	if a.Major != b.Major {
		if a.Major > b.Major {
			return 1
		}
		return -1
	}
	if a.Minor != b.Minor {
		if a.Minor > b.Minor {
			return 1
		}
		return -1
	}
	if a.Patch != b.Patch {
		if a.Patch > b.Patch {
			return 1
		}
		return -1
	}
	return 0
}
