package domain

import (
	"reflect"
	"testing"
)

func TestCognitionMetaRequiresStringDictionary(t *testing.T) {
	for _, encoded := range []string{"", " ", "{", "{} trailing", "null", "[]", `"text"`, "42", "true", `{"type":null}`, `{"type":42}`, `{"type":true}`, `{"type":[]}`, `{"type":{}}`} {
		t.Run(encoded, func(t *testing.T) {
			if _, err := DecodeCognitionMeta(encoded); err == nil {
				t.Fatalf("accepted invalid metadata %q", encoded)
			}
		})
	}
	for _, test := range []struct {
		encoded string
		want    map[string]string
	}{
		{`{}`, map[string]string{}},
		{` { "type":"theorem", "scope":"几何", "note":"" } `, map[string]string{"type": "theorem", "scope": "几何", "note": ""}},
	} {
		meta, err := DecodeCognitionMeta(test.encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(meta, test.want) {
			t.Fatalf("metadata changed: %#v", meta)
		}
	}
}
