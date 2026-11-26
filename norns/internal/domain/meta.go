package domain

import "encoding/json"

// DecodeCognitionMeta validates domain metadata without interpreting its keys.
func DecodeCognitionMeta(encoded string) (map[string]string, error) {
	var object map[string]any
	if err := json.Unmarshal([]byte(encoded), &object); err != nil || object == nil {
		return nil, NewError("INVALID_ARGUMENT", "meta must be a JSON object with string values")
	}
	meta := make(map[string]string, len(object))
	for key, value := range object {
		text, ok := value.(string)
		if !ok {
			return nil, NewError("INVALID_ARGUMENT", "meta must be a JSON object with string values")
		}
		meta[key] = text
	}
	return meta, nil
}
