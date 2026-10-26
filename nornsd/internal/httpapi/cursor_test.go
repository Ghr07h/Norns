package httpapi

import "testing"

func TestCursorBindsRouteAndFilters(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	token, err := encodeCursor(key, "subjects", "idle", []string{"time", "id"})
	if err != nil {
		t.Fatal(err)
	}
	sort, err := decodeCursor(key, token, "subjects", "idle")
	if err != nil || len(sort) != 2 {
		t.Fatalf("decode: %#v %v", sort, err)
	}
	if _, err = decodeCursor(key, token, "queue", "idle"); err == nil {
		t.Fatal("cursor was accepted on another route")
	}
	if _, err = decodeCursor(key, token, "subjects", "using"); err == nil {
		t.Fatal("cursor was accepted with other filters")
	}
}
