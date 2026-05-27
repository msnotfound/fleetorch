package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	repoOwnerRepo  = "msnotfound/fleetorch"
	latestEndpoint = "https://api.github.com/repos/" + repoOwnerRepo + "/releases/latest"
	releaseBase    = "https://github.com/" + repoOwnerRepo + "/releases/download/"
)

func newUpgradeCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Download the latest release and replace this binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doUpgrade(force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Upgrade even if running version is already latest")
	return cmd
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func doUpgrade(force bool) error {
	rel, err := fetchLatest()
	if err != nil {
		return err
	}
	tag := rel.TagName
	if !force && strings.TrimPrefix(tag, "v") == strings.TrimPrefix(Version, "v") && Version != "dev" {
		fmt.Printf("already on %s\n", Version)
		return nil
	}
	fmt.Printf("upgrading: %s → %s\n", Version, tag)

	assetName, archive, err := pickAsset(rel)
	if err != nil {
		return err
	}

	// Download checksums to verify the archive.
	sumsURL := releaseBase + tag + "/checksums.txt"
	sums, err := fetchString(sumsURL)
	if err != nil {
		return fmt.Errorf("fetch checksums: %w", err)
	}
	wantSum, ok := lookupChecksum(sums, assetName)
	if !ok {
		return fmt.Errorf("checksum for %s not in checksums.txt", assetName)
	}

	tmpDir, err := os.MkdirTemp("", "fleetorch-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, assetName)
	if err := downloadTo(archive, archivePath); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	gotSum, err := sha256File(archivePath)
	if err != nil {
		return err
	}
	if gotSum != wantSum {
		return fmt.Errorf("checksum mismatch: got %s, want %s", gotSum, wantSum)
	}

	extracted, err := extractBinary(archivePath, tmpDir)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, _ = filepath.EvalSymlinks(self)

	// Stage the new binary next to the old one (same fs → fast rename).
	staged := self + ".new"
	if err := copyFile(extracted, staged); err != nil {
		return err
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		_ = os.Remove(staged)
		return err
	}

	// Direct rename: works on Linux/macOS and on Windows when the .exe
	// isn't actually locked. If that fails (typical for a running Windows
	// .exe), move the current binary aside first, then move the new one
	// into place. The .old file can't be deleted while we're running, but
	// it gets cleaned up on the next upgrade or by `fleetorch upgrade`'s
	// startup sweep.
	if err := os.Rename(staged, self); err != nil {
		old := self + ".old"
		_ = os.Remove(old) // best-effort cleanup of any previous .old
		if mvErr := os.Rename(self, old); mvErr != nil {
			_ = os.Remove(staged)
			return fmt.Errorf("replace binary at %s: %w (and could not move running binary aside: %v)", self, err, mvErr)
		}
		if err2 := os.Rename(staged, self); err2 != nil {
			// Restore so the user isn't left without a binary.
			_ = os.Rename(old, self)
			_ = os.Remove(staged)
			return fmt.Errorf("replace binary at %s: %w", self, err2)
		}
	}

	// Best-effort sweep: drop any stale .old left from a previous upgrade.
	_ = os.Remove(self + ".old")

	fmt.Printf("upgraded: %s\n", self)
	return nil
}

func fetchLatest() (*ghRelease, error) {
	c := http.Client{Timeout: 30 * time.Second}
	req, _ := http.NewRequest("GET", latestEndpoint, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api: %s", resp.Status)
	}
	var r ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

func pickAsset(r *ghRelease) (name, url string, err error) {
	var osTag string
	switch runtime.GOOS {
	case "linux":
		osTag = "linux"
	case "darwin":
		osTag = "macos"
	case "windows":
		osTag = "windows"
	default:
		return "", "", fmt.Errorf("unsupported OS for upgrade: %s", runtime.GOOS)
	}
	var archTag string
	switch runtime.GOARCH {
	case "amd64":
		archTag = "x86_64"
	case "arm64":
		archTag = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported arch for upgrade: %s", runtime.GOARCH)
	}
	for _, a := range r.Assets {
		if strings.Contains(a.Name, "_"+osTag+"_"+archTag+".") {
			return a.Name, a.BrowserDownloadURL, nil
		}
	}
	return "", "", fmt.Errorf("no asset matched %s/%s in release %s", osTag, archTag, r.TagName)
}

func lookupChecksum(checksums, name string) (string, bool) {
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			return fields[0], true
		}
	}
	return "", false
}

func fetchString(url string) (string, error) {
	c := http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("%s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func downloadTo(url, dst string) error {
	c := http.Client{Timeout: 5 * time.Minute}
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func extractBinary(archivePath, dir string) (string, error) {
	switch {
	case strings.HasSuffix(archivePath, ".zip"):
		return extractZipBinary(archivePath, dir)
	case strings.HasSuffix(archivePath, ".tar.gz"):
		return extractTarGzBinary(archivePath, dir)
	default:
		return "", fmt.Errorf("unknown archive: %s", archivePath)
	}
}

func extractTarGzBinary(archivePath, dir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if filepath.Base(h.Name) == "fleetorch" {
			out := filepath.Join(dir, "fleetorch")
			of, err := os.Create(out)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(of, tr); err != nil {
				of.Close()
				return "", err
			}
			of.Close()
			return out, nil
		}
	}
	return "", fmt.Errorf("fleetorch binary not found inside %s", archivePath)
}

func extractZipBinary(archivePath, dir string) (string, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, file := range zr.File {
		base := filepath.Base(file.Name)
		if base == "fleetorch" || base == "fleetorch.exe" {
			out := filepath.Join(dir, base)
			rc, err := file.Open()
			if err != nil {
				return "", err
			}
			of, err := os.Create(out)
			if err != nil {
				rc.Close()
				return "", err
			}
			if _, err := io.Copy(of, rc); err != nil {
				rc.Close()
				of.Close()
				return "", err
			}
			rc.Close()
			of.Close()
			return out, nil
		}
	}
	return "", fmt.Errorf("fleetorch binary not found inside %s", archivePath)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
