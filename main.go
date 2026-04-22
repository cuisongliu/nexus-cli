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

	"nexus3/internal/httpx"
	"nexus3/internal/nexus"
	"nexus3/internal/npm"
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
	case "upload":
		return runUpload(args[1:])
	case "npm-down":
		return runNpmDown(args[1:])
	case "npm-pack":
		return runNpmPack(args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], rootUsage())
	}
}

func runUpload(args []string) error {
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var repoURL string
	var username string
	var password string
	var workers int
	var insecure bool

	fs.StringVar(&repoURL, "r", "", "Nexus repository URL")
	fs.StringVar(&repoURL, "repo", "", "Nexus repository URL")
	fs.StringVar(&username, "u", "", "Nexus username")
	fs.StringVar(&username, "username", "", "Nexus username")
	fs.StringVar(&password, "p", "", "Nexus password")
	fs.StringVar(&password, "userpass", "", "Nexus password")
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent upload workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, uploadUsage())
	}

	if len(fs.Args()) != 1 {
		return errors.New(uploadUsage())
	}
	if repoURL == "" {
		return errors.New("upload requires -r/--repo\n\n" + uploadUsage())
	}

	return nexus.UploadRepository(context.Background(), fs.Args()[0], nexus.UploadOptions{
		RepoURL:  repoURL,
		Username: username,
		Password: password,
		Workers:  workers,
		Client:   httpx.NewClient(5*time.Minute, insecure),
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	})
}

func runNpmDown(args []string) error {
	fs := flag.NewFlagSet("npm-down", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var workers int
	var insecure bool
	fs.IntVar(&workers, "workers", runtime.NumCPU(), "Concurrent download workers")
	fs.BoolVar(&insecure, "k", false, "Skip HTTPS certificate verification")
	fs.BoolVar(&insecure, "insecure", false, "Skip HTTPS certificate verification")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, npmDownUsage())
	}
	if len(fs.Args()) != 2 {
		return errors.New(npmDownUsage())
	}

	return npm.DownloadTarballs(context.Background(), fs.Args()[0], fs.Args()[1], npm.DownloadOptions{
		Workers: workers,
		Client:  httpx.NewClient(10*time.Minute, insecure),
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
}

func runNpmPack(args []string) error {
	fs := flag.NewFlagSet("npm-pack", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w\n\n%s", err, npmPackUsage())
	}
	if len(fs.Args()) != 2 {
		return errors.New(npmPackUsage())
	}

	return npm.PackTarballs(context.Background(), fs.Args()[0], fs.Args()[1], npm.PackOptions{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
}

func printRootUsage(w io.Writer) {
	fmt.Fprint(w, rootUsage())
}

func rootUsage() string {
	return `Usage:
  nexus3 upload -r <repo> [-u <username>] [-p <password>] [-workers <n>] [-k] <path>
  nexus3 npm-down [-workers <n>] [-k] <package-lock.json|yarn.lock|node_modules> <store_path>
  nexus3 npm-pack <project_path> <store_path>

Commands:
  upload    Recursively upload .jar and .pom files to a Nexus repository
  npm-down  Download npm tarballs referenced by package-lock.json, yarn.lock, or node_modules
  npm-pack  Run npm pack for discovered packages under .pnpm or node_modules
`
}

func uploadUsage() string {
	return `Usage:
  nexus3 upload -r <repo> [-u <username>] [-p <password>] [-workers <n>] [-k] <path>

Example:
  nexus3 upload -r http://127.0.0.1:8081/repository/maven-hosted -u admin -p 123456 ~/.m2/repository
`
}

func npmDownUsage() string {
	return `Usage:
  nexus3 npm-down [-workers <n>] [-k] <package-lock.json|yarn.lock|node_modules> <store_path>

Examples:
  nexus3 npm-down ./package-lock.json ./store
  nexus3 npm-down -k ./package-lock.json ./store
  nexus3 npm-down ./yarn.lock ./store
  nexus3 npm-down ./node_modules ./store
`
}

func npmPackUsage() string {
	return `Usage:
  nexus3 npm-pack <project_path> <store_path>

Example:
  nexus3 npm-pack ./my-project ./store
`
}
