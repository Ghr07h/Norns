package versions

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestEmbeddedVersionsMatchSource(t *testing.T) {
	source, err := os.ReadFile("versions.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if got := Read(); !reflect.DeepEqual(got, want) {
		t.Fatalf("embedded versions = %#v, want %#v", got, want)
	}
}

func TestRejectInvalidVersions(t *testing.T) {
	for _, test := range []struct {
		path  string
		value any
	}{
		{"protocol", "2"},
		{"protocol", 0},
		{"norns.version", "dev"},
		{"norns.version", "1.2"},
		{"norns.version", "01.2.3"},
		{"norns.support_protocol", nil},
		{"norns.database_schema", -1},
		{"nornsd.version", ""},
		{"nornsd.support_protocol", 0},
		{"nornsd.database_schema", 1.5},
		{"nornsd.api_schema", 0},
		{"nornsd.agent_config_schema", 0},
		{"nornsd.plugins.cc", nil},
	} {
		t.Run(fmt.Sprintf("%s/%v", test.path, test.value), func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(test.path, ".")
			node := value
			for _, part := range parts[:len(parts)-1] {
				node = node[part].(map[string]any)
			}
			node[parts[len(parts)-1]] = test.value
			source, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = parse(source); err == nil {
				t.Fatal("invalid version was accepted")
			}
		})
	}
}
