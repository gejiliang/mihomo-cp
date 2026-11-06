package service

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func assertPrivateConfig(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("config = %q, %v; want %q", data, err, want)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions: %v, %v", info, err)
	}
}

func TestWriteConfigFile(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing-wide-permissions"}[existing], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if existing {
				if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteConfigFile(path, []byte("new-secret")); err != nil {
				t.Fatal(err)
			}
			assertPrivateConfig(t, path, "new-secret")
			info, _ := os.Stat(path)
			owner := info.Sys().(*syscall.Stat_t)
			if int(owner.Uid) != os.Geteuid() || int(owner.Gid) != os.Getegid() {
				t.Fatalf("unexpected owner: %v", owner)
			}
			entries, _ := os.ReadDir(filepath.Dir(path))
			if len(entries) != 1 {
				t.Fatalf("temporary files left: %v", entries)
			}
		})
	}
}

func TestWriteConfigFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target"), filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfigFile(link, []byte("replacement")); err == nil {
		t.Fatal("accepted symlink")
	}
	data, _ := os.ReadFile(target)
	info, _ := os.Lstat(link)
	if string(data) != "original" || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("changed symlink or target")
	}
}

func TestWriteConfigFileFailurePreservesOriginal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged directory access checks")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if err := WriteConfigFile(path, []byte("replacement")); err == nil {
		t.Fatal("write unexpectedly succeeded")
	}
	data, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	if string(data) != "original" || info.Mode().Perm() != 0o644 {
		t.Fatal("failed write changed original")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temporary files left: %v", entries)
	}
}
