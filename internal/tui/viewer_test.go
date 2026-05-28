package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "empty is text", data: []byte{}, want: false},
		{name: "plain ASCII", data: []byte("hello world\n"), want: false},
		{name: "valid UTF-8", data: []byte("héllo · 世界"), want: false},
		{name: "null byte at start", data: []byte{0x00, 'a', 'b'}, want: true},
		{name: "null byte in middle", data: append(append([]byte("abc"), 0x00), []byte("def")...), want: true},
		{name: "invalid UTF-8", data: []byte{0xC0, 0xC1, 0xFE}, want: true},
		{
			name: "PNG header",
			data: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R'},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBinary(tt.data); got != tt.want {
				t.Fatalf("isBinary(%v) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "main.go", want: "go"},
		{path: "src/lib/util.ts", want: "typescript"},
		{path: "app.tsx", want: "tsx"},
		{path: "script.py", want: "python"},
		{path: "lib.rs", want: "rust"},
		{path: "Dockerfile", want: "docker"},
		{path: "Makefile", want: "makefile"},
		{path: "build/Dockerfile", want: "docker"},
		// uppercase extension still classifies
		{path: "INDEX.HTML", want: "html"},
		// extension-less unrecognised file → no language
		{path: "README", want: ""},
		// unknown extension → no language
		{path: "notes.unknown", want: ""},
		// .gitignore is treated as text (filepath.Ext returns ".gitignore"
		// because there's no name before the dot)
		{path: ".gitignore", want: ""},
		// markdown returns empty lang (handled via viewerKindMarkdown)
		{path: "README.md", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := detectLanguage(tt.path); got != tt.want {
				t.Fatalf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestClassifyKinds(t *testing.T) {
	tests := []struct {
		path string
		want viewerKind
	}{
		{path: "README.md", want: viewerKindMarkdown},
		{path: "guide.markdown", want: viewerKindMarkdown},
		{path: "main.go", want: viewerKindCode},
		{path: "Dockerfile", want: viewerKindCode},
		{path: "notes.txt", want: viewerKindText},
		{path: ".env", want: viewerKindText},
		{path: "unknown.xyz", want: viewerKindText},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, _ := classify(tt.path)
			if got != tt.want {
				t.Fatalf("classify(%q) kind = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func writeTempFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestNewViewerEachKind(t *testing.T) {
	t.Run("markdown", func(t *testing.T) {
		path := writeTempFile(t, "doc.md", []byte("# Hello\n\nWorld."))
		v := newViewer(path, 80, 24)
		if v == nil {
			t.Fatal("expected non-nil viewer")
		}
		if v.kind != viewerKindMarkdown {
			t.Fatalf("kind = %v, want markdown", v.kind)
		}
		if v.loadErr != nil {
			t.Fatalf("unexpected load error: %v", v.loadErr)
		}
	})

	t.Run("code", func(t *testing.T) {
		path := writeTempFile(t, "main.go", []byte("package main\n\nfunc main() {}\n"))
		v := newViewer(path, 80, 24)
		if v == nil {
			t.Fatal("expected non-nil viewer")
		}
		if v.kind != viewerKindCode {
			t.Fatalf("kind = %v, want code", v.kind)
		}
		if v.lang != "go" {
			t.Fatalf("lang = %q, want go", v.lang)
		}
	})

	t.Run("text", func(t *testing.T) {
		path := writeTempFile(t, "log.txt", []byte("hello\nworld\n"))
		v := newViewer(path, 80, 24)
		if v == nil {
			t.Fatal("expected non-nil viewer")
		}
		if v.kind != viewerKindText {
			t.Fatalf("kind = %v, want text", v.kind)
		}
	})

	t.Run("binary", func(t *testing.T) {
		path := writeTempFile(t, "data.bin", []byte{0xFF, 0x00, 0xAB, 0xCD, 0x00})
		v := newViewer(path, 80, 24)
		if v == nil {
			t.Fatal("expected non-nil viewer")
		}
		if v.kind != viewerKindBinary {
			t.Fatalf("kind = %v, want binary", v.kind)
		}
		out := v.View()
		if !strings.Contains(out, "Binary file") {
			t.Fatalf("binary view did not contain expected message: %q", out)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		v := newViewer(filepath.Join(t.TempDir(), "does-not-exist"), 80, 24)
		if v == nil {
			t.Fatal("expected non-nil viewer")
		}
		if v.loadErr == nil {
			t.Fatal("expected load error")
		}
	})
}

func TestViewerFooterContainsPath(t *testing.T) {
	path := writeTempFile(t, "README.md", []byte("# title\n"))
	v := newViewer(path, 80, 24)
	out := v.View()
	if !strings.Contains(out, "README.md") {
		t.Fatalf("expected footer to contain filename, got: %q", out)
	}
	if !strings.Contains(out, "esc back") {
		t.Fatalf("expected footer help text, got: %q", out)
	}
}

func TestViewerSetSizeRerenders(t *testing.T) {
	path := writeTempFile(t, "main.go", []byte("package main\nfunc main(){}\n"))
	v := newViewer(path, 80, 24)
	v.SetSize(100, 30)
	if v.vp.Width != 100 {
		t.Fatalf("vp.Width = %d, want 100", v.vp.Width)
	}
	if v.vp.Height != 30-viewerFooterRows {
		t.Fatalf("vp.Height = %d, want %d", v.vp.Height, 30-viewerFooterRows)
	}
}
