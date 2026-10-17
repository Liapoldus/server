package caddy

import "testing"

func TestStripStaticMountPrefix(t *testing.T) {
	tests := []struct {
		name, path, prefix, want string
		ok                       bool
	}{
		{name: "root document", path: "/docs", prefix: "/docs", want: "/", ok: true},
		{name: "nested document", path: "/docs/assets/logo.svg", prefix: "/docs", want: "/assets/logo.svg", ok: true},
		{name: "prefix boundary", path: "/docs-old/index.html", prefix: "/docs", ok: false},
		{name: "traversal rejected", path: "/docs/../secrets", prefix: "/docs", ok: false},
		{name: "encoded path rejected", path: "/docs/%2e%2e/secrets", prefix: "/docs", ok: false},
		{name: "root mount", path: "/index.html", prefix: "/", want: "/index.html", ok: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := stripStaticMountPrefix(test.path, test.prefix)
			if got != test.want || ok != test.ok {
				t.Fatalf("stripStaticMountPrefix(%q, %q) = (%q, %t), want (%q, %t)", test.path, test.prefix, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestStaticReleaseAndMountValidation(t *testing.T) {
	if !validReleaseID("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatal("expected lowercase SHA-256 release ID to be accepted")
	}
	for _, value := range []string{"", "sha256:ABC", "sha256:gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg"} {
		if validReleaseID(value) {
			t.Fatalf("invalid release ID accepted: %q", value)
		}
	}
	for _, value := range []string{"/", "/assets", "/assets/"} {
		if !validStaticMountPrefix(value) {
			t.Fatalf("valid mount prefix rejected: %q", value)
		}
	}
	for _, value := range []string{"", "assets", "/a//b", "/a/../b", "/a%2fb", "/a?x=1", "/a\\b"} {
		if validStaticMountPrefix(value) {
			t.Fatalf("invalid mount prefix accepted: %q", value)
		}
	}
}
