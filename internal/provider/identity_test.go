package provider

import (
	"net/url"
	"strings"
	"testing"
)

func FuzzModelKey(f *testing.F) {
	for _, pair := range [][4]string{
		{"a", "b::c", "a::b", "c"},
		{"a:", "c", "a", ":c"},
		{"a%3A", "c", "a:", "c"},
		{"a b", "c", "a+b", "c"},
		{"", "::", "::", ""},
	} {
		f.Add(pair[0], pair[1], pair[2], pair[3])
	}
	f.Fuzz(func(t *testing.T, p1, m1, p2, m2 string) {
		key := ModelKey(p1, m1)
		encoded, model, ok := strings.Cut(key, "::")
		id, err := url.QueryUnescape(encoded)
		if !ok || err != nil || id != p1 || model != m1 {
			t.Fatalf("identity does not round-trip: %q, %q -> %q", p1, m1, key)
		}
		if (key == ModelKey(p2, m2)) != (p1 == p2 && m1 == m2) {
			t.Fatal("distinct identities share a key")
		}
	})
}
