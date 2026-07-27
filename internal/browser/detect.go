package browser

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// DownloadURL is shown to users when no Chrome installation is found.
const DownloadURL = "https://www.google.com/chrome/"

// candidatePaths returns platform-specific absolute locations to probe, in
// priority order.
func candidatePaths() []string {
	switch runtime.GOOS {
	case "windows":
		var paths []string
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
			if base := os.Getenv(env); base != "" {
				paths = append(paths, filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
			}
		}
		return paths
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			filepath.Join(os.Getenv("HOME"), "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
		}
	default: // linux and friends
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/opt/google/chrome/chrome",
		}
	}
}

// pathNames returns the executables to search for on $PATH.
func pathNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"chrome.exe"}
	}
	return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"}
}

// FindChrome locates an installed Chrome/Chromium executable. It returns the
// absolute path and true if found.
func FindChrome() (string, bool) {
	for _, p := range candidatePaths() {
		if isExecutable(p) {
			return p, true
		}
	}
	for _, name := range pathNames() {
		if p, err := exec.LookPath(name); err == nil {
			return p, true
		}
	}
	return "", false
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return true
}
