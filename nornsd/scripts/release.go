package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const product = "nornsd"

const usage = `Usage: go run ./scripts/release.go [--all-platforms] [--output directory]

Builds nornsd for the current platform, or all six Go targets with --all-platforms.
Reads ../versions.json and writes to ../release by default.
`

type options struct {
	allPlatforms bool
	help         bool
	output       string
}

type versions struct {
	Protocol int `json:"protocol"`
	Nornsd   struct {
		Version           string         `json:"version"`
		SupportProtocol   int            `json:"support_protocol"`
		DatabaseSchema    int            `json:"database_schema"`
		APISchema         int            `json:"api_schema"`
		AgentConfigSchema int            `json:"agent_config_schema"`
		Plugins           map[string]int `json:"plugins"`
	} `json:"nornsd"`
}

type target struct {
	os   string
	arch string
}

func (platform target) directory() string {
	arch := platform.arch
	if arch == "amd64" {
		arch = "x64"
	}
	return platform.os + "-" + arch
}

type runCommand func(directory string, environment []string, name string, arguments ...string) error

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run() error {
	selected, err := parseOptions(os.Args[1:])
	if err != nil {
		return err
	}
	if selected.help {
		fmt.Print(usage)
		return nil
	}
	root, err := findRoot()
	if err != nil {
		return err
	}
	return build(root, selected, execute)
}

func findRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "versions.json")); err == nil {
			if _, err := os.Stat(filepath.Join(directory, product, "go.mod")); err == nil {
				return directory, nil
			}
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("run from the repository or %s directory", product)
		}
		directory = parent
	}
}

func parseOptions(arguments []string) (options, error) {
	var selected options
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--help", "-h":
			selected.help = true
		case "--all-platforms":
			selected.allPlatforms = true
		case "--output":
			index++
			if index >= len(arguments) || arguments[index] == "" || strings.HasPrefix(arguments[index], "--") {
				return options{}, fmt.Errorf("--output requires a directory")
			}
			selected.output = arguments[index]
		default:
			return options{}, fmt.Errorf("unknown argument %q\n%s", arguments[index], usage)
		}
	}
	return selected, nil
}

func readVersions(root string) (versions, error) {
	var manifest versions
	source, err := os.ReadFile(filepath.Join(root, "versions.json"))
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(source, &manifest); err != nil {
		return manifest, fmt.Errorf("invalid versions.json: %w", err)
	}
	pattern := regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	if !pattern.MatchString(manifest.Nornsd.Version) {
		return manifest, fmt.Errorf("versions.json: nornsd.version must be a three-part product version")
	}
	for name, value := range map[string]int{
		"protocol":                manifest.Protocol,
		"nornsd.support_protocol": manifest.Nornsd.SupportProtocol,
		"nornsd.database_schema":  manifest.Nornsd.DatabaseSchema,
		"nornsd.api_schema":       manifest.Nornsd.APISchema, "nornsd.agent_config_schema": manifest.Nornsd.AgentConfigSchema, "nornsd.plugins.cc": manifest.Nornsd.Plugins["cc"],
	} {
		if value < 1 {
			return manifest, fmt.Errorf("versions.json: %s must be a positive integer", name)
		}
	}
	for name, value := range manifest.Nornsd.Plugins {
		if value < 1 {
			return manifest, fmt.Errorf("versions.json: nornsd.plugins.%s must be a positive integer", name)
		}
	}

	return manifest, nil
}

func generateVersions(root string, manifest versions) error {
	declarations := fmt.Sprintf("ProductVersion = %q\nDatabaseSchema = %d\nAPISchema = %d\nAgentConfigVersion = %d\nNornsProtocol = %d\nCCPluginVersion = %d\n", manifest.Nornsd.Version, manifest.Nornsd.DatabaseSchema, manifest.Nornsd.APISchema, manifest.Nornsd.AgentConfigSchema, manifest.Nornsd.SupportProtocol, manifest.Nornsd.Plugins["cc"])
	source, err := format.Source([]byte("package version\n\nconst (\n" + declarations + ")\n"))
	if err != nil {
		return err
	}
	filename := filepath.Join(root, product, "internal/version/versions_generated.go")
	current, err := os.ReadFile(filename)
	if err == nil && bytes.Equal(current, source) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(filename, source, 0o644)
}

func build(root string, selected options, command runCommand) error {
	manifest, err := readVersions(root)
	if err != nil {
		return err
	}
	release := selected.output
	if release == "" {
		release = filepath.Join(root, "release")
	}
	release, err = filepath.Abs(release)
	if err != nil {
		return err
	}
	if err := ensureDirectory(release); err != nil {
		return err
	}
	platforms := []target{{runtime.GOOS, runtime.GOARCH}}
	if selected.allPlatforms {
		platforms = []target{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}}
	}
	for _, platform := range platforms {
		if err := ensureDirectory(filepath.Join(release, platform.directory())); err != nil {
			return err
		}
	}
	staging, err := os.MkdirTemp(root, ".release.tmp.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	commit := os.Getenv("VCS_REF")
	if commit == "" {
		git := exec.Command("git", "rev-parse", "--short=12", "HEAD")
		git.Dir = root
		if output, err := git.Output(); err == nil {
			commit = strings.TrimSpace(string(output))
		}
	}
	if commit == "" {
		commit = "unknown"
	}
	buildTime := os.Getenv("BUILD_TIME")
	if buildTime == "" {
		buildTime = time.Now().UTC().Format(time.RFC3339)
	}
	if err := generateVersions(root, manifest); err != nil {
		return err
	}
	commands := []string{product}
	if err := command(filepath.Join(root, product), []string{"CGO_ENABLED=0"}, "go", "generate", "./internal/assets"); err != nil {
		return err
	}
	linkerFlags := fmt.Sprintf("-s -w -X 'github.com/Ghr07h/norns/%s/internal/version.Commit=%s' -X 'github.com/Ghr07h/norns/%s/internal/version.BuildTime=%s'", product, commit, product, buildTime)
	for _, platform := range platforms {
		for _, executable := range commands {
			filename := executable
			if platform.os == "windows" {
				filename += ".exe"
			}
			output := filepath.Join(staging, platform.directory(), filename)
			if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
				return err
			}
			environment := []string{"CGO_ENABLED=0", "GOOS=" + platform.os, "GOARCH=" + platform.arch}
			if err := command(filepath.Join(root, product), environment, "go", "build", "-trimpath", "-ldflags", linkerFlags, "-o", output, "./cmd/"+executable); err != nil {
				return err
			}
		}
	}
	for _, platform := range platforms {
		if err := publish(filepath.Join(staging, platform.directory()), filepath.Join(release, platform.directory())); err != nil {
			return err
		}
	}
	if err := writeChecksums(release, staging); err != nil {
		return err
	}
	fmt.Printf("release: wrote %s to %s and updated SHA256SUMS\n", product, release)
	return nil
}

func execute(directory string, environment []string, name string, arguments ...string) error {
	fmt.Printf("release: %s: %s %s\n", filepath.Base(directory), name, strings.Join(arguments, " "))
	command := exec.Command(name, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func ensureDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		return os.MkdirAll(directory, 0o755)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("refusing to replace non-directory or symbolic link %s", directory)
	}
	return nil
}

func publish(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	current, err := os.ReadDir(destination)
	if err != nil {
		return err
	}
	for _, entry := range current {
		name := entry.Name()
		owned := name == product || name == product+".exe"
		if owned {
			if err := os.RemoveAll(filepath.Join(destination, name)); err != nil {
				return err
			}
		}
	}
	for _, entry := range entries {
		if err := os.Rename(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func writeChecksums(release, staging string) error {
	var contents strings.Builder
	err := filepath.WalkDir(release, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || entry.Name() == "SHA256SUMS" || entry.Name() == ".gitkeep" {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported release artifact %s", path)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		digest := sha256.New()
		if _, err := io.Copy(digest, input); err != nil {
			return err
		}
		relative, err := filepath.Rel(release, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(&contents, "%x  ./%s\n", digest.Sum(nil), filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return err
	}
	temporary := filepath.Join(staging, "SHA256SUMS")
	if err := os.WriteFile(temporary, []byte(contents.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(release, "SHA256SUMS"))
}
