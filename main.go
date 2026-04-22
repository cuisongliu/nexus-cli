package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"nexus-cli/internal/httpx"
	nx2 "nexus-cli/internal/nexus2"
	nx3 "nexus-cli/internal/nexus3"
	"nexus-cli/internal/npm"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printRootUsage(os.Stdout)
		return nil
	}

	switch args[0] {
	case "-h", "--help", "help":
		printRootUsage(os.Stdout)
		return nil
	case "npm-cache":
		return runNpmCache(args[1:])
	case "npm-pkg":
		return runNpmPkg(args[1:])
	case "backup-download":
		return runBackupDownload(args[1:])
	case "backup-upload":
		return runBackupUpload(args[1:])
	case "component-download":
		return runComponentDownload(args[1:])
	case "component-upload":
		return runComponentUpload(args[1:])
	case "nexus2-backup-upload":
		return runNexus2BackupUpload(args[1:])
	case "nexus2-component-upload":
		return runNexus2ComponentUpload(args[1:])
	case "nexus2-backup-download":
		return runNexus2BackupDownload(args[1:])
	case "nexus2-component-download":
		return runNexus2ComponentDownload(args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], rootUsage())
	}
}

func runNpmCache(args []string) error {
	fs := flag.NewFlagSet("npm-cache", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var workers int
	var insecure bool
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent download workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, npmCacheUsage())
	}
	if len(fs.Args()) != 2 {
		return errors.New(npmCacheUsage())
	}

	return npm.DownloadTarballs(context.Background(), fs.Args()[0], fs.Args()[1], npm.DownloadOptions{
		Workers: workers,
		Client:  httpx.NewClient(10*time.Minute, insecure),
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
}

func runNpmPkg(args []string) error {
	fs := flag.NewFlagSet("npm-pkg", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, npmPkgUsage())
	}
	if len(fs.Args()) != 2 {
		return errors.New(npmPkgUsage())
	}

	return npm.PackTarballs(context.Background(), fs.Args()[0], fs.Args()[1], npm.PackOptions{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
}

func runBackupDownload(args []string) error {
	fs := flag.NewFlagSet("backup-download", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var formatRaw string
	var baseURL string
	var repository string
	var repoURL string
	var username string
	var password string
	var workers int
	var insecure bool

	fs.StringVar(&formatRaw, "format", "", "Repository format: maven or npm")
	fs.StringVar(&baseURL, "base-url", "", "Nexus base URL, for example https://host")
	fs.StringVar(&repository, "repository", "", "Nexus repository name")
	fs.StringVar(&repoURL, "repo-url", "", "Nexus repository URL, for example https://host/repository/maven-releases")
	fs.StringVar(&username, "u", "", "Nexus username")
	fs.StringVar(&username, "username", "", "Nexus username")
	fs.StringVar(&password, "p", "", "Nexus password")
	fs.StringVar(&password, "userpass", "", "Nexus password")
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent download workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, backupDownloadUsage())
	}
	if len(fs.Args()) != 1 {
		return errors.New(backupDownloadUsage())
	}

	format, err := nx3.ParseFormat(formatRaw)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, backupDownloadUsage())
	}

	client, target, err := newNexus3Client(baseURL, repository, repoURL, username, password, insecure, 10*time.Minute)
	if err != nil {
		return err
	}

	return nx3.DownloadRepositoryBackup(context.Background(), client, nx3.DownloadRepositoryOptions{
		Target:    target,
		Format:    format,
		OutputDir: fs.Args()[0],
		Workers:   workers,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})
}

func runBackupUpload(args []string) error {
	fs := flag.NewFlagSet("backup-upload", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var formatRaw string
	var baseURL string
	var repository string
	var repoURL string
	var username string
	var password string
	var workers int
	var insecure bool

	fs.StringVar(&formatRaw, "format", "", "Repository format: maven or npm")
	fs.StringVar(&baseURL, "base-url", "", "Nexus base URL, for example https://host")
	fs.StringVar(&repository, "repository", "", "Nexus repository name")
	fs.StringVar(&repoURL, "repo-url", "", "Nexus repository URL, for example https://host/repository/maven-releases")
	fs.StringVar(&username, "u", "", "Nexus username")
	fs.StringVar(&username, "username", "", "Nexus username")
	fs.StringVar(&password, "p", "", "Nexus password")
	fs.StringVar(&password, "userpass", "", "Nexus password")
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent upload workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, backupUploadUsage())
	}
	if len(fs.Args()) != 1 {
		return errors.New(backupUploadUsage())
	}

	format, err := nx3.ParseFormat(formatRaw)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, backupUploadUsage())
	}

	client, target, err := newNexus3Client(baseURL, repository, repoURL, username, password, insecure, 10*time.Minute)
	if err != nil {
		return err
	}

	return nx3.UploadBackup(context.Background(), client, nx3.UploadOptions{
		Target:    target,
		Format:    format,
		InputPath: fs.Args()[0],
		Workers:   workers,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})
}

func runComponentDownload(args []string) error {
	fs := flag.NewFlagSet("component-download", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var formatRaw string
	var baseURL string
	var repository string
	var repoURL string
	var username string
	var password string
	var workers int
	var insecure bool
	var groupID string
	var artifactID string
	var name string
	var version string

	fs.StringVar(&formatRaw, "format", "", "Repository format: maven or npm")
	fs.StringVar(&baseURL, "base-url", "", "Nexus base URL, for example https://host")
	fs.StringVar(&repository, "repository", "", "Nexus repository name")
	fs.StringVar(&repoURL, "repo-url", "", "Nexus repository URL, for example https://host/repository/maven-releases")
	fs.StringVar(&username, "u", "", "Nexus username")
	fs.StringVar(&username, "username", "", "Nexus username")
	fs.StringVar(&password, "p", "", "Nexus password")
	fs.StringVar(&password, "userpass", "", "Nexus password")
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent download workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")
	fs.StringVar(&groupID, "group-id", "", "Maven groupId")
	fs.StringVar(&artifactID, "artifact-id", "", "Maven artifactId")
	fs.StringVar(&name, "name", "", "npm package name")
	fs.StringVar(&version, "version", "", "Component version")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, componentDownloadUsage())
	}
	if len(fs.Args()) != 1 {
		return errors.New(componentDownloadUsage())
	}

	format, err := nx3.ParseFormat(formatRaw)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, componentDownloadUsage())
	}

	client, target, err := newNexus3Client(baseURL, repository, repoURL, username, password, insecure, 10*time.Minute)
	if err != nil {
		return err
	}

	switch format {
	case nx3.FormatMaven:
		if groupID == "" || artifactID == "" || version == "" {
			return errors.New("component-download for maven requires --group-id, --artifact-id and --version\n\n" + componentDownloadUsage())
		}
		return nx3.DownloadMavenComponent(context.Background(), client, nx3.DownloadMavenComponentOptions{
			Target:     target,
			GroupID:    groupID,
			ArtifactID: artifactID,
			Version:    version,
			OutputDir:  fs.Args()[0],
			Workers:    workers,
			Stdout:     os.Stdout,
			Stderr:     os.Stderr,
		})
	case nx3.FormatNPM:
		if name == "" || version == "" {
			return errors.New("component-download for npm requires --name and --version\n\n" + componentDownloadUsage())
		}
		return nx3.DownloadNPMComponent(context.Background(), client, nx3.DownloadNPMComponentOptions{
			Target:    target,
			Name:      name,
			Version:   version,
			OutputDir: fs.Args()[0],
			Workers:   workers,
			Stdout:    os.Stdout,
			Stderr:    os.Stderr,
		})
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func runComponentUpload(args []string) error {
	fs := flag.NewFlagSet("component-upload", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var formatRaw string
	var baseURL string
	var repository string
	var repoURL string
	var username string
	var password string
	var workers int
	var insecure bool

	fs.StringVar(&formatRaw, "format", "", "Repository format: maven or npm")
	fs.StringVar(&baseURL, "base-url", "", "Nexus base URL, for example https://host")
	fs.StringVar(&repository, "repository", "", "Nexus repository name")
	fs.StringVar(&repoURL, "repo-url", "", "Nexus repository URL, for example https://host/repository/maven-releases")
	fs.StringVar(&username, "u", "", "Nexus username")
	fs.StringVar(&username, "username", "", "Nexus username")
	fs.StringVar(&password, "p", "", "Nexus password")
	fs.StringVar(&password, "userpass", "", "Nexus password")
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent upload workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, componentUploadUsage())
	}
	if len(fs.Args()) != 1 {
		return errors.New(componentUploadUsage())
	}

	format, err := nx3.ParseFormat(formatRaw)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, componentUploadUsage())
	}

	client, target, err := newNexus3Client(baseURL, repository, repoURL, username, password, insecure, 10*time.Minute)
	if err != nil {
		return err
	}

	return nx3.UploadBackup(context.Background(), client, nx3.UploadOptions{
		Target:    target,
		Format:    format,
		InputPath: fs.Args()[0],
		Workers:   workers,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})
}

func runNexus2BackupUpload(args []string) error {
	fs := flag.NewFlagSet("nexus2-backup-upload", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var formatRaw string
	var baseURL string
	var repository string
	var repoURL string
	var username string
	var password string
	var workers int
	var insecure bool

	fs.StringVar(&formatRaw, "format", "", "Repository format: maven or npm")
	fs.StringVar(&baseURL, "base-url", "", "Nexus2 base URL, for example https://host")
	fs.StringVar(&repository, "repository", "", "Nexus2 repository name")
	fs.StringVar(&repoURL, "repo-url", "", "Nexus2 repository URL, for example https://host/content/repositories/releases")
	fs.StringVar(&username, "u", "", "Nexus2 username")
	fs.StringVar(&username, "username", "", "Nexus2 username")
	fs.StringVar(&password, "p", "", "Nexus2 password")
	fs.StringVar(&password, "userpass", "", "Nexus2 password")
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent upload workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, nexus2BackupUploadUsage())
	}
	if len(fs.Args()) != 1 {
		return errors.New(nexus2BackupUploadUsage())
	}

	format, err := nx3.ParseFormat(formatRaw)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, nexus2BackupUploadUsage())
	}

	client, target, err := newNexus2Client(baseURL, repository, repoURL, username, password, insecure, 10*time.Minute)
	if err != nil {
		return err
	}

	return nx2.ImportBackup(context.Background(), client, nx2.ImportOptions{
		Target:    target,
		Format:    format,
		InputPath: fs.Args()[0],
		Workers:   workers,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})
}

func runNexus2ComponentUpload(args []string) error {
	fs := flag.NewFlagSet("nexus2-component-upload", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var formatRaw string
	var baseURL string
	var repository string
	var repoURL string
	var username string
	var password string
	var workers int
	var insecure bool

	fs.StringVar(&formatRaw, "format", "", "Repository format: maven or npm")
	fs.StringVar(&baseURL, "base-url", "", "Nexus2 base URL, for example https://host")
	fs.StringVar(&repository, "repository", "", "Nexus2 repository name")
	fs.StringVar(&repoURL, "repo-url", "", "Nexus2 repository URL, for example https://host/content/repositories/releases")
	fs.StringVar(&username, "u", "", "Nexus2 username")
	fs.StringVar(&username, "username", "", "Nexus2 username")
	fs.StringVar(&password, "p", "", "Nexus2 password")
	fs.StringVar(&password, "userpass", "", "Nexus2 password")
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent upload workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, nexus2ComponentUploadUsage())
	}
	if len(fs.Args()) != 1 {
		return errors.New(nexus2ComponentUploadUsage())
	}

	format, err := nx3.ParseFormat(formatRaw)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, nexus2ComponentUploadUsage())
	}

	client, target, err := newNexus2Client(baseURL, repository, repoURL, username, password, insecure, 10*time.Minute)
	if err != nil {
		return err
	}

	return nx2.ImportBackup(context.Background(), client, nx2.ImportOptions{
		Target:    target,
		Format:    format,
		InputPath: fs.Args()[0],
		Workers:   workers,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})
}

func runNexus2BackupDownload(args []string) error {
	fs := flag.NewFlagSet("nexus2-backup-download", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var formatRaw string
	var baseURL string
	var repository string
	var repoURL string
	var username string
	var password string
	var workers int
	var insecure bool

	fs.StringVar(&formatRaw, "format", "", "Repository format: maven or npm")
	fs.StringVar(&baseURL, "base-url", "", "Nexus2 base URL, for example https://host")
	fs.StringVar(&repository, "repository", "", "Nexus2 repository name")
	fs.StringVar(&repoURL, "repo-url", "", "Nexus2 repository URL, for example https://host/content/repositories/releases")
	fs.StringVar(&username, "u", "", "Nexus2 username")
	fs.StringVar(&username, "username", "", "Nexus2 username")
	fs.StringVar(&password, "p", "", "Nexus2 password")
	fs.StringVar(&password, "userpass", "", "Nexus2 password")
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent download workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, nexus2BackupDownloadUsage())
	}
	if len(fs.Args()) != 1 {
		return errors.New(nexus2BackupDownloadUsage())
	}

	format, err := nx3.ParseFormat(formatRaw)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, nexus2BackupDownloadUsage())
	}

	client, target, err := newNexus2Client(baseURL, repository, repoURL, username, password, insecure, 10*time.Minute)
	if err != nil {
		return err
	}

	return nx2.DownloadRepositoryBackup(context.Background(), client, nx2.DownloadRepositoryOptions{
		Target:    target,
		Format:    format,
		OutputDir: fs.Args()[0],
		Workers:   workers,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})
}

func runNexus2ComponentDownload(args []string) error {
	fs := flag.NewFlagSet("nexus2-component-download", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var formatRaw string
	var baseURL string
	var repository string
	var repoURL string
	var username string
	var password string
	var workers int
	var insecure bool
	var groupID string
	var artifactID string
	var name string
	var version string

	fs.StringVar(&formatRaw, "format", "", "Repository format: maven or npm")
	fs.StringVar(&baseURL, "base-url", "", "Nexus2 base URL, for example https://host")
	fs.StringVar(&repository, "repository", "", "Nexus2 repository name")
	fs.StringVar(&repoURL, "repo-url", "", "Nexus2 repository URL, for example https://host/content/repositories/releases")
	fs.StringVar(&username, "u", "", "Nexus2 username")
	fs.StringVar(&username, "username", "", "Nexus2 username")
	fs.StringVar(&password, "p", "", "Nexus2 password")
	fs.StringVar(&password, "userpass", "", "Nexus2 password")
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent download workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")
	fs.StringVar(&groupID, "group-id", "", "Maven groupId")
	fs.StringVar(&artifactID, "artifact-id", "", "Maven artifactId")
	fs.StringVar(&name, "name", "", "npm package name")
	fs.StringVar(&version, "version", "", "Component version")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, nexus2ComponentDownloadUsage())
	}
	if len(fs.Args()) != 1 {
		return errors.New(nexus2ComponentDownloadUsage())
	}

	format, err := nx3.ParseFormat(formatRaw)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, nexus2ComponentDownloadUsage())
	}

	client, target, err := newNexus2Client(baseURL, repository, repoURL, username, password, insecure, 10*time.Minute)
	if err != nil {
		return err
	}

	switch format {
	case nx3.FormatMaven:
		if groupID == "" || artifactID == "" || version == "" {
			return errors.New("nexus2-component-download for maven requires --group-id, --artifact-id and --version\n\n" + nexus2ComponentDownloadUsage())
		}
		return nx2.DownloadMavenComponent(context.Background(), client, nx2.DownloadMavenComponentOptions{
			Target:     target,
			GroupID:    groupID,
			ArtifactID: artifactID,
			Version:    version,
			OutputDir:  fs.Args()[0],
			Workers:    workers,
			Stdout:     os.Stdout,
			Stderr:     os.Stderr,
		})
	case nx3.FormatNPM:
		if name == "" || version == "" {
			return errors.New("nexus2-component-download for npm requires --name and --version\n\n" + nexus2ComponentDownloadUsage())
		}
		return nx2.DownloadNPMComponent(context.Background(), client, nx2.DownloadNPMComponentOptions{
			Target:    target,
			Name:      name,
			Version:   version,
			OutputDir: fs.Args()[0],
			Workers:   workers,
			Stdout:    os.Stdout,
			Stderr:    os.Stderr,
		})
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func newNexus3Client(baseURL, repository, repoURL, username, password string, insecure bool, timeout time.Duration) (*nx3.Client, nx3.Target, error) {
	target, err := nx3.ResolveTarget(baseURL, repository, repoURL)
	if err != nil {
		return nil, nx3.Target{}, err
	}

	client, err := nx3.NewClient(target.BaseURL, username, password, httpx.NewClient(timeout, insecure))
	if err != nil {
		return nil, nx3.Target{}, err
	}
	return client, target, nil
}

func newNexus2Client(baseURL, repository, repoURL, username, password string, insecure bool, timeout time.Duration) (*nx2.Client, nx2.Target, error) {
	target, err := nx2.ResolveTarget(baseURL, repository, repoURL)
	if err != nil {
		return nil, nx2.Target{}, err
	}

	client, err := nx2.NewClient(target.BaseURL, username, password, httpx.NewClient(timeout, insecure))
	if err != nil {
		return nil, nx2.Target{}, err
	}
	return client, target, nil
}

func printRootUsage(w io.Writer) {
	fmt.Fprint(w, rootUsage())
}

func rootUsage() string {
	return `Usage:
  Local npm tools:
    nexus-cli npm-cache [-workers <n>] [-k] <package-lock.json|yarn.lock|node_modules> <store_path>
    nexus-cli npm-pkg <project_path> <store_path>

  Nexus3 commands:
    nexus-cli backup-download --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <output_dir>
    nexus-cli backup-upload --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <input_path>
    nexus-cli component-download --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [selectors] [-u <username>] [-p <password>] [-workers <n>] [-k] <output_dir>
    nexus-cli component-upload --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <input_path>

  Nexus2 import commands:
    nexus-cli nexus2-backup-upload --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <input_path>
    nexus-cli nexus2-component-upload --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <input_path>

  Nexus2 export commands:
    nexus-cli nexus2-backup-download --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <output_dir>
    nexus-cli nexus2-component-download --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [selectors] [-u <username>] [-p <password>] [-workers <n>] [-k] <output_dir>

Local npm tools:
  npm-cache           Cache npm tarballs from package-lock.json, yarn.lock, or node_modules
  npm-pkg             Run npm pack for discovered packages under .pnpm or node_modules

Nexus3 commands:
  backup-download     Download a full Maven or npm Nexus3 repository into a backup directory
  backup-upload       Restore a Maven or npm backup directory into a Nexus3 repository
  component-download  Download a specific Maven or npm component from Nexus3
  component-upload    Upload a specific Maven or npm component backup into Nexus3

Nexus2 import commands:
  nexus2-backup-upload     Import Nexus3 backup data into a Nexus2 repository
  nexus2-component-upload  Import a Nexus3 component backup or npm tarball into Nexus2

Nexus2 export commands:
  nexus2-backup-download     Export Maven or npm data from Nexus2 into Nexus3 backup format
  nexus2-component-download  Export a specific Maven or npm component from Nexus2
`
}

func npmCacheUsage() string {
	return `Usage:
  nexus-cli npm-cache [-workers <n>] [-k] <package-lock.json|yarn.lock|node_modules> <store_path>

Examples:
  nexus-cli npm-cache ./package-lock.json ./store
  nexus-cli npm-cache -k ./package-lock.json ./store
  nexus-cli npm-cache ./yarn.lock ./store
  nexus-cli npm-cache ./node_modules ./store
`
}

func npmPkgUsage() string {
	return `Usage:
  nexus-cli npm-pkg <project_path> <store_path>

Example:
  nexus-cli npm-pkg ./my-project ./store
`
}

func backupDownloadUsage() string {
	return `Usage:
  nexus-cli backup-download --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <output_dir>

Examples:
  nexus-cli backup-download --format maven --base-url https://host --repository maven-releases -u admin -p 123456 ./backup
  nexus-cli backup-download --format npm --repo-url https://host/repository/npm-hosted -u admin -p 123456 ./backup
`
}

func backupUploadUsage() string {
	return `Usage:
  nexus-cli backup-upload --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <input_path>

Examples:
  nexus-cli backup-upload --format maven --base-url https://host --repository maven-releases -u admin -p 123456 ./backup
  nexus-cli backup-upload --format npm --repo-url https://host/repository/npm-hosted -u admin -p 123456 ./backup
`
}

func componentDownloadUsage() string {
	return `Usage:
  nexus-cli component-download --format maven (--base-url <url> --repository <name> | --repo-url <url>) --group-id <groupId> --artifact-id <artifactId> --version <version> [-u <username>] [-p <password>] [-workers <n>] [-k] <output_dir>
  nexus-cli component-download --format npm (--base-url <url> --repository <name> | --repo-url <url>) --name <package> --version <version> [-u <username>] [-p <password>] [-workers <n>] [-k] <output_dir>

Examples:
  nexus-cli component-download --format maven --repo-url https://host/repository/maven-releases --group-id com.example --artifact-id demo --version 1.0.0 -u admin -p 123456 ./component
  nexus-cli component-download --format npm --base-url https://host --repository npm-hosted --name @scope/demo --version 1.0.0 -u admin -p 123456 ./component
`
}

func componentUploadUsage() string {
	return `Usage:
  nexus-cli component-upload --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <input_path>

Examples:
  nexus-cli component-upload --format maven --repo-url https://host/repository/maven-releases -u admin -p 123456 ./component-backup
  nexus-cli component-upload --format npm --base-url https://host --repository npm-hosted -u admin -p 123456 ./package.tgz
`
}

func nexus2BackupUploadUsage() string {
	return `Usage:
  nexus-cli nexus2-backup-upload --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <input_path>

Examples:
  nexus-cli nexus2-backup-upload --format maven --repo-url https://host/content/repositories/releases -u admin -p 123456 ./backup
  nexus-cli nexus2-backup-upload --format npm --base-url https://host --repository npm-hosted -u admin -p 123456 ./backup
`
}

func nexus2ComponentUploadUsage() string {
	return `Usage:
  nexus-cli nexus2-component-upload --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <input_path>

Examples:
  nexus-cli nexus2-component-upload --format maven --repo-url https://host/content/repositories/releases -u admin -p 123456 ./component
  nexus-cli nexus2-component-upload --format npm --base-url https://host --repository npm-hosted -u admin -p 123456 ./package.tgz
`
}

func nexus2BackupDownloadUsage() string {
	return `Usage:
  nexus-cli nexus2-backup-download --format <maven|npm> (--base-url <url> --repository <name> | --repo-url <url>) [-u <username>] [-p <password>] [-workers <n>] [-k] <output_dir>

Examples:
  nexus-cli nexus2-backup-download --format maven --repo-url https://host/content/repositories/releases -u admin -p 123456 ./backup
  nexus-cli nexus2-backup-download --format npm --base-url https://host --repository npm-hosted -u admin -p 123456 ./backup
`
}

func nexus2ComponentDownloadUsage() string {
	return `Usage:
  nexus-cli nexus2-component-download --format maven (--base-url <url> --repository <name> | --repo-url <url>) --group-id <groupId> --artifact-id <artifactId> --version <version> [-u <username>] [-p <password>] [-workers <n>] [-k] <output_dir>
  nexus-cli nexus2-component-download --format npm (--base-url <url> --repository <name> | --repo-url <url>) --name <package> --version <version> [-u <username>] [-p <password>] [-workers <n>] [-k] <output_dir>

Examples:
  nexus-cli nexus2-component-download --format maven --repo-url https://host/content/repositories/releases --group-id com.example --artifact-id demo --version 1.0.0 -u admin -p 123456 ./component
  nexus-cli nexus2-component-download --format npm --base-url https://host --repository npm-hosted --name demo-pkg --version 1.0.0 -u admin -p 123456 ./component
`
}
