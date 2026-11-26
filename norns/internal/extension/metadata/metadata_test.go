package metadata

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const valid = `{"name":"lookup","version":"0.1.0","description":"Lookup data","requirements":[]}`

func TestReleaseFilePathValidation(t *testing.T) {
	for _, file := range []ReleaseFile{{OriginFile: "config.example.yaml"}, {OriginFile: ".env.example"}, {OriginFile: "configs/example.yaml", TargetFile: "lookup.example.yaml"}, {OriginFile: "./config.example.yaml", TargetFile: ""}} {
		if err := file.Validate(); err != nil {
			t.Fatalf("valid release file rejected: %+v", file)
		}
	}
	for _, origin := range []string{"", "../secret", "dir/../secret", "/etc/passwd", `C:\secret`, `dir\..\secret`, "dir//file"} {
		if (ReleaseFile{OriginFile: origin, TargetFile: "config.yaml"}).Validate() == nil {
			t.Fatalf("invalid origin accepted: %q", origin)
		}
	}
	for _, target := range []string{"../file", "dir/file", `dir\file`, "/file", "file.", "a\x00b"} {
		if (ReleaseFile{OriginFile: "config.yaml", TargetFile: target}).Validate() == nil {
			t.Fatalf("invalid target accepted: %q", target)
		}
	}
	for _, test := range []struct {
		file ReleaseFile
		want string
	}{
		{ReleaseFile{OriginFile: "config.example.yaml"}, "norns.exts.lookup.config.example.yaml"},
		{ReleaseFile{OriginFile: "configs/original.yaml", TargetFile: "custom.yaml"}, "norns.exts.lookup.custom.yaml"},
		{ReleaseFile{OriginFile: "agent.yaml"}, "norns.exts.lookup.agent.yaml"},
	} {
		if test.file.ValidateFor("lookup") != nil || test.file.PublishedName("lookup") != test.want {
			t.Fatalf("wrong prefixed filename: %+v", test)
		}
	}
	if (ReleaseFile{OriginFile: "json"}).ValidateFor("meta") == nil {
		t.Fatal("shared metadata filename collision accepted")
	}
	if (ReleaseFile{OriginFile: "config.yaml", TargetFile: strings.Repeat("a", 250)}).ValidateFor("lookup") == nil {
		t.Fatal("prefixed filename exceeds the file name limit")
	}
	if (ReleaseFile{OriginFile: "configs/example.yaml"}).TargetName() != "example.yaml" {
		t.Fatal("default target should be the basename")
	}
}

func TestMetadataValidation(t *testing.T) {
	item, err := ParseSource([]byte(valid), "lookup")
	if err != nil || item.Name != "lookup" || item.Requirements == nil || len(item.Requirements) != 0 {
		t.Fatalf("item=%+v err=%v", item, err)
	}
	for _, source := range []string{
		`null`, `[]`, `{}`, valid + `{}`,
		strings.Replace(valid, `"lookup"`, `"../lookup"`, 1),
		strings.Replace(valid, `"0.1.0"`, `null`, 1),
		strings.Replace(valid, `"0.1.0"`, `1`, 1),
		strings.Replace(valid, `"Lookup data"`, `" "`, 1),
		strings.Replace(valid, `"requirements":[]`, `"requirements":null`, 1),
		strings.Replace(valid, `"requirements":[]`, `"requirements":[""]`, 1),
		strings.Replace(valid, `"requirements":[]`, `"requirements":[1]`, 1),
		strings.Replace(valid, `"requirements":[]`, `"requirements":[null]`, 1),
		strings.Replace(valid, `"requirements":[]`, `"other":[]`, 1),
		strings.Replace(valid, `"name":"lookup"`, `"name":"other","name":"lookup"`, 1),
		strings.Replace(valid, `"name":"lookup"`, `"NAME":"lookup"`, 1),
		strings.Replace(valid, `"requirements":[]`, `"requirements":[],"script":"run"`, 1),
		strings.Replace(valid, `Lookup data`, strings.Repeat("中", 2049), 1),
		strings.Repeat("[", 32) + "0" + strings.Repeat("]", 32),
		valid + strings.Repeat(" ", MaxSourceBytes),
	} {
		t.Run(source[:min(len(source), 80)], func(t *testing.T) {
			if _, err := ParseSource([]byte(source), "lookup"); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
	if _, err := ParseSource([]byte(valid), "other"); err == nil {
		t.Fatal("metadata name must match the selected extension")
	}
	for _, source := range []string{`null`, `{}`, "[" + valid + "," + valid + "]", "[" + valid + ",{}]", "[]{}", "[]" + strings.Repeat(" ", MaxFileBytes)} {
		if _, err := ParseCatalog([]byte(source)); err == nil {
			t.Fatal("invalid catalog accepted")
		}
	}
	if items, err := ParseCatalog([]byte("[]")); err != nil || items == nil {
		t.Fatalf("empty catalog: %+v %v", items, err)
	}
}

func TestMetadataFileRestrictions(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "metadata.json")
	if err := os.WriteFile(filename, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := ReadFile(filename, MaxSourceBytes); err != nil || string(data) != valid {
		t.Fatalf("read metadata: %s %v", data, err)
	}
	if _, err := ReadFile(filename, 8); err == nil {
		t.Fatal("oversized file accepted")
	}
	if _, err := ReadFile(filepath.Dir(filename), MaxSourceBytes); err == nil {
		t.Fatal("directory accepted")
	}
	if runtime.GOOS != "windows" {
		link := filename + ".link"
		if err := os.Symlink(filename, link); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadFile(link, MaxSourceBytes); err == nil {
			t.Fatal("symbolic link accepted")
		}
	}
}
