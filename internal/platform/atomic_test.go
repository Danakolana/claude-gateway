package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAtomicOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	if err := WriteFileAtomic(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("two"), 0o600); err != nil {
		t.Fatalf("overwrite: %v (GOOS=%s)", err, runtime.GOOS)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "two" {
		t.Fatalf("got %q", got)
	}
}

func TestReplaceFileMissingDest(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, "x.tmp")
	dest := filepath.Join(dir, "x")
	if err := os.WriteFile(tmp, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceFile(tmp, dest); err != nil {
		t.Fatal(err)
	}
}
