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
	"sort"
	"strings"
	"time"

	"github.com/Ghr07h/norns/norns/internal/extension/metadata"
	"github.com/Ghr07h/norns/norns/internal/extension/releasefiles"
)

const product = "norns"

const usage = `Usage: go run ./scripts/release.go [--all-platforms | --target os/arch] [--with-ext name]... [--output directory]

Builds norns for the current platform, or all six Go targets with --all-platforms.
Extensions are excluded by default; repeat --with-ext to select them.
Repeat --target to build selected targets (darwin, linux, windows; amd64, arm64).
Reads ../versions.json and writes to ../release by default.
`

var extensionNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

type options struct {
	allPlatforms bool
	targets      []target
	help         bool
	output       string
	extensions   []string
	publishFrom  string
}

type versions struct {
	Protocol int `json:"protocol"`
	Norns    struct {
		Version         string `json:"version"`
		SupportProtocol int    `json:"support_protocol"`
		DatabaseSchema  int    `json:"database_schema"`
	} `json:"norns"`
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
	if selected.publishFrom != "" {
		if selected.output == "" || selected.allPlatforms || len(selected.targets) != 0 || len(selected.extensions) != 0 {
			return fmt.Errorf("--publish-from requires --output and cannot be combined with build options")
		}
		return publishStaged(selected.publishFrom, selected.output)
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
		case "--target":
			index++
			if index >= len(arguments) {
				return options{}, fmt.Errorf("--target requires os/arch")
			}
			parts := strings.Split(arguments[index], "/")
			if len(parts) != 2 || (parts[0] != "darwin" && parts[0] != "linux" && parts[0] != "windows") || (parts[1] != "amd64" && parts[1] != "arm64") {
				return options{}, fmt.Errorf("unsupported --target %q", arguments[index])
			}
			platform := target{parts[0], parts[1]}
			duplicate := false
			for _, existing := range selected.targets {
				duplicate = duplicate || existing == platform
			}
			if !duplicate {
				selected.targets = append(selected.targets, platform)
			}
		case "--with-ext":
			index++
			if index >= len(arguments) || !extensionNamePattern.MatchString(arguments[index]) {
				return options{}, fmt.Errorf("--with-ext requires an extension name (lowercase letters, digits and hyphens, starting with a letter)")
			}
			selected.extensions = append(selected.extensions, arguments[index])
		case "--output":
			index++
			if index >= len(arguments) || arguments[index] == "" || strings.HasPrefix(arguments[index], "--") {
				return options{}, fmt.Errorf("--output requires a directory")
			}
			selected.output = arguments[index]
		case "--publish-from":
			index++
			if index >= len(arguments) || arguments[index] == "" || strings.HasPrefix(arguments[index], "--") {
				return options{}, fmt.Errorf("--publish-from requires a staging directory")
			}
			selected.publishFrom = arguments[index]
		default:
			return options{}, fmt.Errorf("unknown argument %q\n%s", arguments[index], usage)
		}
	}
	if selected.allPlatforms && len(selected.targets) != 0 {
		return options{}, fmt.Errorf("--target cannot be combined with --all-platforms")
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
	if !pattern.MatchString(manifest.Norns.Version) {
		return manifest, fmt.Errorf("versions.json: norns.version must be a three-part product version")
	}
	for name, value := range map[string]int{
		"protocol":               manifest.Protocol,
		"norns.support_protocol": manifest.Norns.SupportProtocol,
		"norns.database_schema":  manifest.Norns.DatabaseSchema,
	} {
		if value < 1 {
			return manifest, fmt.Errorf("versions.json: %s must be a positive integer", name)
		}
	}

	return manifest, nil
}

func generateVersions(root string, manifest versions) error {
	declarations := fmt.Sprintf("ProductVersion = %q\nProtocolVersion = %d\nSchemaVersion = %d\n", manifest.Norns.Version, manifest.Norns.SupportProtocol, manifest.Norns.DatabaseSchema)
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

func buildPackages(root string, extensions []string) ([]string, error) {
	packages := []string{"./cmd/norns"}
	seen := make(map[string]bool)
	for _, name := range extensions {
		if !extensionNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid extension name %q", name)
		}
		if seen[name] {
			continue
		}
		pkg := "./internal/custom_extensions/" + name + "/cmd/norns-" + name
		entry := filepath.Join(root, product, pkg, "main.go")
		info, err := os.Stat(entry)
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("unknown extension %q: expected %s", name, entry)
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("extension %q entry must be a regular Go file: %s", name, entry)
		}
		packages = append(packages, pkg)
		seen[name] = true
	}
	return packages, nil
}

func build(root string, selected options, command runCommand) error {
	manifest, err := readVersions(root)
	if err != nil {
		return err
	}
	packages, err := buildPackages(root, selected.extensions)
	if err != nil {
		return err
	}
	catalog := make([]metadata.Metadata, 0, len(packages)-1)
	for _, pkg := range packages[1:] {
		name := strings.TrimPrefix(filepath.Base(pkg), "norns-")
		filename := filepath.Join(root, product, "internal/custom_extensions", name, "metadata.json")
		data, err := metadata.ReadFile(filename, metadata.MaxSourceBytes)
		if err != nil {
			return fmt.Errorf("extension %q: cannot read metadata.json", name)
		}
		item, err := metadata.ParseSource(data, name)
		if err != nil {
			return fmt.Errorf("extension %q: invalid metadata.json", name)
		}
		catalog = append(catalog, item)
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].Name < catalog[j].Name })
	files, err := releasefiles.Plan(filepath.Join(root, product, "internal/custom_extensions"), catalog)
	if err != nil {
		return err
	}
	catalogJSON, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return err
	}
	if _, err := metadata.ParseCatalog(catalogJSON); err != nil {
		return fmt.Errorf("combined extension metadata exceeds supported limits")
	}
	catalogJSON = append(catalogJSON, '\n')
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
	platforms := selected.targets
	if len(platforms) == 0 {
		platforms = []target{{runtime.GOOS, runtime.GOARCH}}
	}
	if selected.allPlatforms {
		platforms = []target{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}}
	}
	for _, platform := range platforms {
		if err := ensureDirectory(filepath.Join(release, platform.directory())); err != nil {
			return err
		}
	}
	staging, err := os.MkdirTemp(release, ".release.tmp.")
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
	linkerFlags := fmt.Sprintf("-s -w -X 'github.com/Ghr07h/norns/%s/internal/version.Commit=%s' -X 'github.com/Ghr07h/norns/%s/internal/version.BuildTime=%s'", product, commit, product, buildTime)
	for _, platform := range platforms {
		for _, pkg := range packages {
			executable := filepath.Base(pkg)
			filename := executable
			if platform.os == "windows" {
				filename += ".exe"
			}
			output := filepath.Join(staging, platform.directory(), filename)
			if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
				return err
			}
			environment := []string{"CGO_ENABLED=0", "GOOS=" + platform.os, "GOARCH=" + platform.arch}
			if err := command(filepath.Join(root, product), environment, "go", "build", "-trimpath", "-ldflags", linkerFlags, "-o", output, pkg); err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(staging, platform.directory(), metadata.Filename), catalogJSON, 0o644); err != nil {
			return err
		}
		if err := releasefiles.Stage(files, filepath.Join(staging, platform.directory())); err != nil {
			return err
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
	return releasefiles.Publish(source, destination)
}

// publishStaged is used by the root coordinator after every product has built.
func publishStaged(source, destination string) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	if source == destination {
		return fmt.Errorf("staging and release directories must differ")
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		targetDir := filepath.Join(destination, entry.Name())
		if err := ensureDirectory(targetDir); err != nil {
			return err
		}
		if err := publish(filepath.Join(source, entry.Name()), targetDir); err != nil {
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
		if path != release && entry.IsDir() && strings.HasPrefix(entry.Name(), ".release.tmp.") {
			return filepath.SkipDir
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
