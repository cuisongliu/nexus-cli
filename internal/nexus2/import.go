package nexus2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	nx3 "nexus3/internal/nexus3"
)

type ImportOptions struct {
	Target    Target
	Format    nx3.Format
	InputPath string
	Workers   int
	Stdout    io.Writer
	Stderr    io.Writer
}

func ImportBackup(ctx context.Context, client *Client, opts ImportOptions) error {
	opts = normalizeImportOptions(opts)

	switch opts.Format {
	case nx3.FormatMaven:
		return importMaven(ctx, client, opts)
	case nx3.FormatNPM:
		return importNPM(ctx, client, opts)
	default:
		return fmt.Errorf("unsupported format %q", opts.Format)
	}
}

func importMaven(ctx context.Context, client *Client, opts ImportOptions) error {
	items, err := nx3.ResolveMavenUploadInputs(opts.InputPath)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no maven assets found in %s", opts.InputPath)
	}

	slices.SortFunc(items, func(a, b nx3.UploadFile) int {
		return strings.Compare(a.RepositoryPath, b.RepositoryPath)
	})
	fmt.Fprintf(opts.Stdout, "found %d maven assets to import into Nexus2\n", len(items))

	errs := make(chan error, len(items))
	jobs := make(chan nx3.UploadFile)
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
		return fmt.Errorf("%d maven imports failed: %w", len(uploadErrs), errors.Join(uploadErrs...))
	}

	fmt.Fprintln(opts.Stdout, "all maven imports completed")
	return nil
}

func importNPM(ctx context.Context, client *Client, opts ImportOptions) error {
	tarballs, err := nx3.ResolveNPMUploadInputs(opts.InputPath)
	if err != nil {
		return err
	}
	if len(tarballs) == 0 {
		return fmt.Errorf("no npm tarballs found in %s", opts.InputPath)
	}

	slices.Sort(tarballs)
	fmt.Fprintf(opts.Stdout, "found %d npm tarballs to import into Nexus2\n", len(tarballs))

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
				if err := client.PublishNPMTarball(ctx, opts.Target, tarball); err != nil {
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
		return fmt.Errorf("%d npm imports failed: %w", len(uploadErrs), errors.Join(uploadErrs...))
	}

	fmt.Fprintln(opts.Stdout, "all npm imports completed")
	return nil
}

func normalizeImportOptions(opts ImportOptions) ImportOptions {
	if opts.Workers < 1 {
		opts.Workers = 1
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	return opts
}
