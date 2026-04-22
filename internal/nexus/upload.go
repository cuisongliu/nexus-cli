package nexus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"nexus-cli/internal/httpx"
)

type UploadOptions struct {
	RepoURL  string
	Username string
	Password string
	Workers  int
	Client   *http.Client
	Stdout   io.Writer
	Stderr   io.Writer
}

func UploadRepository(ctx context.Context, repoPath string, opts UploadOptions) error {
	var err error
	opts, err = normalizeUploadOptions(opts)
	if err != nil {
		return err
	}

	info, err := os.Stat(repoPath)
	if err != nil {
		return fmt.Errorf("stat repo path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("repo path must be a directory: %s", repoPath)
	}

	files, err := collectUploadFiles(repoPath)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Fprintf(opts.Stdout, "no .jar or .pom files found under %s\n", repoPath)
		return nil
	}

	sort.Strings(files)
	fmt.Fprintf(opts.Stdout, "found %d files to upload\n", len(files))

	workerCount := opts.Workers
	if workerCount < 1 {
		workerCount = runtime.NumCPU()
		if workerCount < 1 {
			workerCount = 1
		}
	}
	if workerCount > len(files) {
		workerCount = len(files)
	}

	jobs := make(chan string)
	errs := make(chan error, len(files))
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				if err := uploadFile(ctx, repoPath, path, opts); err != nil {
					errs <- err
				}
			}
		}()
	}

	for _, file := range files {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			close(errs)
			return ctx.Err()
		case jobs <- file:
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
		return fmt.Errorf("%d uploads failed: %w", len(uploadErrs), errors.Join(uploadErrs...))
	}

	fmt.Fprintln(opts.Stdout, "all uploads completed")
	return nil
}

func normalizeUploadOptions(opts UploadOptions) (UploadOptions, error) {
	if opts.Client == nil {
		opts.Client = httpx.NewClient(5*time.Minute, false)
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	repoURL, err := httpx.NormalizeBaseURL(opts.RepoURL)
	if err != nil {
		return UploadOptions{}, fmt.Errorf("invalid repo url: %w", err)
	}
	opts.RepoURL = repoURL
	return opts, nil
}

func collectUploadFiles(repoPath string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".jar") || strings.HasSuffix(path, ".pom") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk repo path: %w", err)
	}
	return files, nil
}

func uploadFile(ctx context.Context, repoPath, filePath string, opts UploadOptions) error {
	relativePath, err := relativeUploadPath(repoPath, filePath)
	if err != nil {
		return err
	}

	targetURL := opts.RepoURL + "/" + relativePath
	fmt.Fprintf(opts.Stdout, "uploading %s\n", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer file.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, file)
	if err != nil {
		return fmt.Errorf("create request for %s: %w", filePath, err)
	}
	req.SetBasicAuth(opts.Username, opts.Password)

	resp, err := opts.Client.Do(req)
	if err != nil {
		return fmt.Errorf("upload %s: %w", filePath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("upload %s: unexpected status %s: %s", filePath, resp.Status, strings.TrimSpace(string(body)))
	}

	fmt.Fprintf(opts.Stdout, "uploaded %s\n", filePath)
	return nil
}

func relativeUploadPath(repoPath, filePath string) (string, error) {
	rel, err := filepath.Rel(repoPath, filePath)
	if err != nil {
		return "", fmt.Errorf("relative path for %s: %w", filePath, err)
	}
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "../") || rel == ".." {
		return "", fmt.Errorf("file %s is outside repo path %s", filePath, repoPath)
	}
	return rel, nil
}
