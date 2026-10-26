package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var domainNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type Domain struct {
	Name                string
	DeciderDescription  string
	DeciderExtraInfo    string
	ExplorerDescription string
	ExplorerExtraInfo   string
}

type domainMeta struct {
	DomainName                  string `json:"domain_name"`
	DomainDeciderDescription    string `json:"domain_decider_description"`
	DomainDeciderExtraInfoFile  string `json:"domain_decider_extra_info_file"`
	DomainExplorerDescription   string `json:"domain_explorer_description"`
	DomainExplorerExtraInfoFile string `json:"domain_explorer_extra_info_file"`
}

func LoadDomain(dir string) (Domain, error) {
	var meta domainMeta
	file, err := os.Open(filepath.Join(dir, "meta.json"))
	if err != nil {
		return Domain{}, fmt.Errorf("cannot read NORNS_CUSTOM_DOMAIN_DIR meta.json: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&meta); err != nil {
		return Domain{}, fmt.Errorf("invalid NORNS_CUSTOM_DOMAIN_DIR meta.json: %w", err)
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("multiple JSON values are not allowed")
		}
		return Domain{}, fmt.Errorf("invalid NORNS_CUSTOM_DOMAIN_DIR meta.json: %w", err)
	}
	meta.DomainName = strings.TrimSpace(meta.DomainName)
	meta.DomainDeciderDescription = strings.TrimSpace(meta.DomainDeciderDescription)
	meta.DomainExplorerDescription = strings.TrimSpace(meta.DomainExplorerDescription)
	if !domainNamePattern.MatchString(meta.DomainName) {
		return Domain{}, fmt.Errorf("domain_name must match %s", domainNamePattern.String())
	}
	if meta.DomainDeciderDescription == "" || meta.DomainExplorerDescription == "" {
		return Domain{}, fmt.Errorf("domain_decider_description and domain_explorer_description must not be empty")
	}
	deciderInfo, err := readDomainExtraInfo(dir, meta.DomainDeciderExtraInfoFile, "domain_decider_extra_info_file")
	if err != nil {
		return Domain{}, err
	}
	explorerInfo, err := readDomainExtraInfo(dir, meta.DomainExplorerExtraInfoFile, "domain_explorer_extra_info_file")
	if err != nil {
		return Domain{}, err
	}
	return Domain{
		Name:                meta.DomainName,
		DeciderDescription:  meta.DomainDeciderDescription,
		DeciderExtraInfo:    deciderInfo,
		ExplorerDescription: meta.DomainExplorerDescription,
		ExplorerExtraInfo:   explorerInfo,
	}, nil
}

func readDomainExtraInfo(dir, name, field string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("%s must be a non-empty relative file path", field)
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve NORNS_CUSTOM_DOMAIN_DIR: %w", err)
	}
	target, err := filepath.EvalSymlinks(filepath.Join(dir, name))
	if err != nil {
		return "", fmt.Errorf("cannot resolve %s: %w", field, err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s must stay within NORNS_CUSTOM_DOMAIN_DIR", field)
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s must reference a readable regular file", field)
	}
	b, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", field, err)
	}
	return string(b), nil
}
