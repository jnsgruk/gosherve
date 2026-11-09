package server

import (
	"net/http"
	"testing"
)

func TestAddVaryPreservesExistingTokens(t *testing.T) {
	header := http.Header{"Vary": {"Origin, accept-encoding", "Cookie"}}
	addVary(header, "Accept-Encoding")
	if got := header.Values("Vary"); len(got) != 2 || got[0] != "Origin, accept-encoding" || got[1] != "Cookie" {
		t.Fatalf("existing Vary values changed: %q", got)
	}
	addVary(header, "Accept-Language")
	if got := header.Values("Vary"); len(got) != 3 || got[2] != "Accept-Language" {
		t.Fatalf("missing Vary token was not appended: %q", got)
	}
}

func TestNegotiateContentEncodingParameters(t *testing.T) {
	available := map[string]selectedRepresentation{"gzip": {}, "br": {}, identityEncoding: {}}
	tests := []struct {
		name   string
		header []string
		want   string
	}{
		{"empty parameters", []string{"gzip;;", "br;q=0"}, "gzip"},
		{"whitespace and case", []string{" GZIP ; Q = 1 , BR ; q = 0.5 "}, "gzip"},
		{"ignored parameters", []string{"gzip;level=6;q=1", "br;q=0.5"}, "gzip"},
		{"last quality parameter wins", []string{"gzip;q=0;q=1", "br;q=0.5"}, "gzip"},
		{"duplicate encoding keeps highest quality", []string{"gzip;q=1,gzip;q=0,br;q=0.5"}, "gzip"},
		{"invalid quality", []string{"gzip;q=invalid,br;q=0,identity;q=0"}, ""},
		{"empty tokens", []string{",;q=0,,gzip,,"}, "gzip"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := negotiateContentEncoding(test.header, available)
			if got != test.want || ok != (test.want != "") {
				t.Fatalf("got (%q, %t), want (%q, %t)", got, ok, test.want, test.want != "")
			}
		})
	}
}
