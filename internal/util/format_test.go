package util_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/WangYihang/Proxy-Verifier/internal/util"
)

func TestParseRecordXmap(t *testing.T) {
	proto, host, port := util.ParseRecordXmap([]string{"1.2.3.4"})
	if proto != "http" {
		t.Errorf("proto = %q, want http", proto)
	}
	if host != "1.2.3.4" {
		t.Errorf("host = %q, want 1.2.3.4", host)
	}
	if port != 8080 {
		t.Errorf("port = %d, want 8080", port)
	}
}

func TestParseRecordCsvUnknownProtocolDefaultPort(t *testing.T) {
	// An unparseable port with an unknown protocol falls back to 8080.
	proto, host, port := util.ParseRecordCsv([]string{"weird", "10.0.0.1", "not-a-number"})
	if proto != "weird" || host != "10.0.0.1" || port != 8080 {
		t.Errorf("got (%q, %q, %d), want (weird, 10.0.0.1, 8080)", proto, host, port)
	}
}

func TestDetectFileFormat(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	tests := []struct {
		name       string
		path       string
		wantFormat string
		wantErr    bool
	}{
		{"csv three columns", write("c.csv", "http,1.2.3.4,8080\n"), "csv", false},
		{"xmap single column", write("x.txt", "1.2.3.4\n"), "xmap", false},
		{"unknown two columns", write("b.txt", "a,b\n"), "", true},
		{"empty file", write("e.txt", ""), "", true},
		{"missing file", filepath.Join(dir, "nope.txt"), "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, err := util.DetectFileFormat(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected an error, got format %q", format)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if format != tt.wantFormat {
				t.Errorf("format = %q, want %q", format, tt.wantFormat)
			}
		})
	}
}
