package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClaudeDesktopPathsIncludeOSHints(t *testing.T) {
	p3 := ClaudeDesktop3PConfigPaths()
	if len(p3) == 0 {
		t.Fatal("no 3p paths")
	}
	pc := ClaudeDesktopConsumerConfigPaths()
	if len(pc) == 0 {
		t.Fatal("no consumer paths")
	}
	joined3 := strings.Join(p3, "\n")
	joinedC := strings.Join(pc, "\n")
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(joined3, `Claude-3p`) {
			t.Fatalf("windows 3p=%v", p3)
		}
		if !strings.Contains(joinedC, string(filepath.Separator)+"Claude"+string(filepath.Separator)) &&
			!strings.HasSuffix(filepath.Dir(pc[0]), "Claude") {
			t.Fatalf("windows consumer=%v", pc)
		}
		// Current official 3P dir is LocalAppData; Roaming is legacy.
		if !strings.Contains(joined3, filepath.Join("Claude-3p", desktopConfigFile)) {
			t.Fatalf("windows 3p missing config file: %v", p3)
		}
	case "darwin":
		if !strings.Contains(joined3, "Application Support") {
			t.Fatalf("darwin should include Application Support: %v", p3)
		}
	default:
		if !strings.Contains(joined3, filepath.Join(".config", "Claude-3p")) &&
			!strings.Contains(joined3, "Claude-3p") {
			t.Fatalf("linux 3p=%v", p3)
		}
	}
	dir, err := GatewayDataDir()
	if err != nil || dir == "" {
		t.Fatalf("data dir: %v %q", err, dir)
	}
}

func TestWindows3PPathsPreferLocalAppDataOverRoaming(t *testing.T) {
	e := desktopPathEnv{
		GOOS:         "windows",
		Home:         `C:\Users\dev`,
		AppData:      `C:\Users\dev\AppData\Roaming`,
		LocalAppData: `C:\Users\dev\AppData\Local`,
	}
	paths := productConfigPaths(e, "Claude-3p")
	if len(paths) < 2 {
		t.Fatalf("want native current+legacy, got %v", paths)
	}
	joined := strings.Join(paths, "\n")
	if !strings.Contains(joined, filepath.Join("Local", "Claude-3p")) {
		t.Fatalf("missing LocalAppData current path: %v", paths)
	}
	if !strings.Contains(joined, filepath.Join("Roaming", "Claude-3p")) {
		t.Fatalf("missing Roaming legacy path: %v", paths)
	}
	localIdx, roamIdx := -1, -1
	for i, p := range paths {
		if strings.Contains(p, filepath.Join("Local", "Claude-3p")) && !strings.Contains(p, "Packages") {
			localIdx = i
		}
		if strings.Contains(p, filepath.Join("Roaming", "Claude-3p")) && !strings.Contains(p, "Packages") {
			roamIdx = i
		}
	}
	if localIdx < 0 || roamIdx < 0 || localIdx > roamIdx {
		t.Fatalf("LocalAppData should rank before Roaming: %v", paths)
	}
}

func TestDarwin3PPathsPreferApplicationSupport(t *testing.T) {
	e := desktopPathEnv{
		GOOS:          "darwin",
		Home:          "/Users/dev",
		XDGConfigHome: "/Users/dev/.config",
	}
	paths := productConfigPaths(e, "Claude-3p")
	if len(paths) < 1 {
		t.Fatal(paths)
	}
	if !strings.Contains(paths[0], "Application Support") {
		t.Fatalf("primary darwin path should be Application Support, got %v", paths)
	}
}

func TestLinux3PPathsHonorXDGConfigHome(t *testing.T) {
	e := desktopPathEnv{
		GOOS:          "linux",
		Home:          "/home/dev",
		XDGConfigHome: "/home/dev/.xdg-config",
	}
	paths := productConfigPaths(e, "Claude-3p")
	if len(paths) < 1 {
		t.Fatal(paths)
	}
	if !strings.Contains(paths[0], filepath.Join(".xdg-config", "Claude-3p")) {
		t.Fatalf("primary linux path should follow XDG_CONFIG_HOME, got %v", paths)
	}
	joined := strings.Join(paths, "\n")
	if !strings.Contains(joined, filepath.Join(".config", "Claude-3p")) {
		t.Fatalf("should still list default ~/.config fallback: %v", paths)
	}
}

func TestDiscoverPrefersConfigLibraryOverEmptyNewerLayout(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "Roaming", "Claude-3p")
	current := filepath.Join(root, "Local", "Claude-3p")
	if err := os.MkdirAll(filepath.Join(legacy, "configLibrary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "configLibrary", "_meta.json"), []byte(`{"appliedId":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Newer layout dir exists but has no config yet.
	if err := os.MkdirAll(current, 0o700); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		filepath.Join(current, desktopConfigFile),
		filepath.Join(legacy, desktopConfigFile),
	}
	got := ""
	for _, p := range paths {
		if desktopConfigExists(p) {
			got = p
			break
		}
	}
	if got != paths[0] {
		// Current dir exists, so it wins — leftover Roaming must not steal apply.
		t.Fatalf("newer layout dir should win when it exists, got %s", got)
	}
}

func TestDesktopConfigExistsFromLibraryOnly(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "configLibrary")
	if err := os.MkdirAll(lib, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "_meta.json"), []byte(`{"appliedId":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, desktopConfigFile)
	if !desktopConfigExists(p) {
		t.Fatal("configLibrary/_meta.json should count as Desktop state")
	}
}

func TestMSIXRoamingPathIncludedWhenPackageExists(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "Packages", "Claude_pzs8sxrjxfjjc")
	if err := os.MkdirAll(pkg, 0o700); err != nil {
		t.Fatal(err)
	}
	dirs := msixProductDirs(root, "Claude-3p")
	if len(dirs) != 1 {
		t.Fatalf("expected roaming candidate only, got %v", dirs)
	}
	if !strings.HasSuffix(dirs[0], filepath.Join("LocalCache", "Roaming", "Claude-3p")) {
		t.Fatalf("roaming=%v", dirs)
	}
}

func TestMSIXLocalPathRanksFirstWhenPresent(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "Packages", "Claude_pzs8sxrjxfjjc")
	local := filepath.Join(pkg, "LocalCache", "Local", "Claude-3p")
	if err := os.MkdirAll(local, 0o700); err != nil {
		t.Fatal(err)
	}
	dirs := msixProductDirs(root, "Claude-3p")
	if len(dirs) < 2 {
		t.Fatalf("want local+roaming, got %v", dirs)
	}
	if !strings.HasSuffix(dirs[0], filepath.Join("LocalCache", "Local", "Claude-3p")) {
		t.Fatalf("local should rank first: %v", dirs)
	}
}
