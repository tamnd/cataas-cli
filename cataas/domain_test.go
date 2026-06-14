package cataas

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the domain wiring, which need no network.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "cataas" {
		t.Errorf("Scheme = %q, want cataas", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "cataas" {
		t.Errorf("Identity.Binary = %q, want cataas", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"abc123", "cat", "abc123"},
		{"dkN9o0F3kMzj64ZM", "cat", "dkN9o0F3kMzj64ZM"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify(\"\") should return an error")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("cat", "abc123")
	want := "https://" + Host + "/cat/abc123"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "abc")
	if err == nil {
		t.Error("Locate with unknown type should return error")
	}
}
