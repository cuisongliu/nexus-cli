package npm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"nexus3/internal/httpx"
)

type DownloadOptions struct {
	Workers int
	Client  *http.Client
	Stdout  io.Writer
	Stderr  io.Writer
}

func DownloadTarballs(ctx context.Context, sourcePath, storePath string, opts DownloadOptions) error {
	opts = normalizeDownloadOptions(opts)

	if err := os.MkdirAll(storePath, 0o755); err != nil {
		return fmt.Errorf("create store path: %w", err)
	}

	urls, err := CollectResolvedURLs(sourcePath)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		fmt.Fprintln(opts.Stdout, "no npm tarball URLs found")
		return nil
	}

	fileTargets, err := buildDownloadTargets(urls)
	if err != nil {
		return err
	}
	sort.Slice(fileTargets, func(i, j int) bool {
		return fileTargets[i].Filename < fileTargets[j].Filename
	})
	fmt.Fprintf(opts.Stdout, "found %d tarballs to download\n", len(fileTargets))

	workerCount := opts.Workers
	if workerCount < 1 {
		workerCount = runtime.NumCPU()
		if workerCount < 1 {
			workerCount = 1
		}
	}
	if workerCount > len(fileTargets) {
		workerCount = len(fileTargets)
	}

	jobs := make(chan downloadTarget)
	errs := make(chan error, len(fileTargets))
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if err := downloadOne(ctx, job, storePath, opts); err != nil {
					errs <- err
				}
			}
		}()
	}

	for _, job := range fileTargets {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			close(errs)
			return ctx.Err()
		case jobs <- job:
		}
	}
	close(jobs)
	wg.Wait()
	close(errs)

	var downloadErrs []error
	for err := range errs {
		downloadErrs = append(downloadErrs, err)
	}

	if len(downloadErrs) > 0 {
		return fmt.Errorf("%d downloads failed: %w", len(downloadErrs), errors.Join(downloadErrs...))
	}

	fmt.Fprintln(opts.Stdout, "all downloads completed")
	return nil
}

type downloadTarget struct {
	URL      string
	Filename string
}

func normalizeDownloadOptions(opts DownloadOptions) DownloadOptions {
	if opts.Client == nil {
		opts.Client = httpx.NewClient(10*time.Minute, false)
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	return opts
}

func buildDownloadTargets(urls []string) ([]downloadTarget, error) {
	seen := make(map[string]struct{}, len(urls))
	targets := make([]downloadTarget, 0, len(urls))

	for _, rawURL := range urls {
		filename, err := filenameFromURL(rawURL)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[filename]; ok {
			continue
		}
		seen[filename] = struct{}{}
		targets = append(targets, downloadTarget{
			URL:      rawURL,
			Filename: filename,
		})
	}
	return targets, nil
}

func downloadOne(ctx context.Context, target downloadTarget, storePath string, opts DownloadOptions) error {
	requestURL, err := httpx.NormalizeRequestURL(target.URL)
	if err != nil {
		return err
	}

	destination := filepath.Join(storePath, target.Filename)
	if _, err := os.Stat(destination); err == nil {
		fmt.Fprintf(opts.Stdout, "skip existing %s\n", target.Filename)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", destination, err)
	}

	fmt.Fprintf(opts.Stdout, "downloading %s\n", requestURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("create request for %s: %w", requestURL, err)
	}

	resp, err := opts.Client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", requestURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("download %s: unexpected status %s: %s", requestURL, resp.Status, strings.TrimSpace(string(body)))
	}

	tempPath := destination + ".part"
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", tempPath, err)
	}

	copyErr := func() error {
		_, err := io.Copy(file, resp.Body)
		if err != nil {
			return fmt.Errorf("write %s: %w", tempPath, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close %s: %w", tempPath, err)
		}
		if err := os.Rename(tempPath, destination); err != nil {
			return fmt.Errorf("rename %s: %w", destination, err)
		}
		return nil
	}()

	if copyErr != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return copyErr
	}

	fmt.Fprintf(opts.Stdout, "downloaded %s\n", target.Filename)
	return nil
}

func filenameFromURL(rawURL string) (string, error) {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse url %s: %w", rawURL, err)
	}
	filename := path.Base(parsed.Path)
	if filename == "." || filename == "/" || filename == "" {
		return "", fmt.Errorf("cannot derive filename from url: %s", rawURL)
	}
	return filename, nil
}
