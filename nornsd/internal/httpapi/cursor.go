package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

type cursorPayload struct {
	Version     int      `json:"v"`
	Route       string   `json:"route"`
	FiltersHash string   `json:"filters_hash"`
	Sort        []string `json:"sort"`
}

func filterHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
func encodeCursor(key []byte, route, filters string, sort []string) (string, error) {
	payload, err := json.Marshal(cursorPayload{1, route, filterHash(filters), sort})
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func decodeCursor(key []byte, token, route, filters string) ([]string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid cursor")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, errors.New("invalid cursor signature")
	}
	var value cursorPayload
	if json.Unmarshal(payload, &value) != nil || value.Version != 1 || value.Route != route || value.FiltersHash != filterHash(filters) {
		return nil, errors.New("cursor does not match query")
	}
	return value.Sort, nil
}
