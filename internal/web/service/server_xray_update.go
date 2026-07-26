package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

const (
	xrayReleasesURL      = "https://api.github.com/repos/XTLS/Xray-core/releases"
	xrayVersionsCacheTTL = 15 * time.Minute
	maxXrayReleasesBytes = 4 << 20
	maxXrayArchiveBytes  = 200 << 20
	maxXrayBinaryBytes   = 200 << 20
	maxXrayDigestBytes   = 64 << 10
	minXrayVersionMajor  = 26
	minXrayVersionMinor  = 6
	minXrayVersionPatch  = 27
)

type cachedXrayVersions struct {
	versions  []string
	fetchedAt time.Time
}

type xrayRelease struct {
	TagName string `json:"tag_name"`
}

// GetXrayVersionsCached limits GitHub API traffic and serves the last valid
// result during a transient fetch failure.
func (s *ServerService) GetXrayVersionsCached() ([]string, error) {
	s.versionsCacheMu.Lock()
	cache := s.versionsCache
	s.versionsCacheMu.Unlock()
	if cache != nil && time.Since(cache.fetchedAt) <= xrayVersionsCacheTTL {
		return cache.versions, nil
	}

	versions, err := s.GetXrayVersions()
	if err != nil {
		if cache != nil {
			logger.Warning("GetXrayVersionsCached: serving stale list:", err)
			return cache.versions, nil
		}
		return nil, err
	}

	s.versionsCacheMu.Lock()
	s.versionsCache = &cachedXrayVersions{versions: versions, fetchedAt: time.Now()}
	s.versionsCacheMu.Unlock()
	return versions, nil
}

func (s *ServerService) GetXrayVersions() ([]string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, xrayReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.settingService.NewProxiedHTTPClient(10 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxXrayReleasesBytes))
		var apiError struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &apiError) == nil && apiError.Message != "" {
			return nil, fmt.Errorf("GitHub API error: %s", apiError.Message)
		}
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, resp.Status)
	}

	var releases []xrayRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxXrayReleasesBytes)).Decode(&releases); err != nil {
		return nil, err
	}

	versions := make([]string, 0, len(releases))
	for _, release := range releases {
		if supportedXrayVersion(release.TagName) {
			versions = append(versions, release.TagName)
		}
	}
	return versions, nil
}

func supportedXrayVersion(version string) bool {
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	patch, errPatch := strconv.Atoi(parts[2])
	if errMajor != nil || errMinor != nil || errPatch != nil {
		return false
	}
	if major != minXrayVersionMajor {
		return major > minXrayVersionMajor
	}
	if minor != minXrayVersionMinor {
		return minor > minXrayVersionMinor
	}
	return patch >= minXrayVersionPatch
}

func xrayReleaseAsset() (fileName, archiveBinaryName string, err error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return "", "", fmt.Errorf("xray update is unsupported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return "Xray-linux-64.zip", "xray", nil
}

func (s *ServerService) downloadXray(version string) (string, error) {
	fileName, _, err := xrayReleaseAsset()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://github.com/XTLS/Xray-core/releases/download/%s/%s", version, fileName)
	client := s.settingService.NewProxiedHTTPClient(60 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download xray: unexpected HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxXrayArchiveBytes {
		return "", fmt.Errorf("download xray: archive exceeds %d bytes", maxXrayArchiveBytes)
	}

	file, err := os.CreateTemp("", "xray-*.zip")
	if err != nil {
		return "", err
	}
	archivePath := file.Name()
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(archivePath)
		}
	}()

	n, err := io.Copy(file, io.LimitReader(resp.Body, maxXrayArchiveBytes+1))
	if err != nil {
		return "", err
	}
	if n > maxXrayArchiveBytes {
		return "", fmt.Errorf("download xray: archive exceeds %d bytes", maxXrayArchiveBytes)
	}

	want, err := s.fetchXrayDigestSHA256(client, url+".dgst")
	if err != nil {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); !strings.EqualFold(got, want) {
		return "", fmt.Errorf("xray update aborted: SHA-256 mismatch (expected %s, got %s)", want, got)
	}

	complete = true
	return archivePath, nil
}

func (s *ServerService) fetchXrayDigestSHA256(client *http.Client, digestURL string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, digestURL, nil)
	if err != nil {
		return "", fmt.Errorf("download xray checksum: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download xray checksum: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download xray checksum: unexpected HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxXrayDigestBytes))
	if err != nil {
		return "", fmt.Errorf("download xray checksum: %w", err)
	}
	return parseXrayDigestSHA256(raw)
}

func parseXrayDigestSHA256(digest []byte) (string, error) {
	for line := range strings.SplitSeq(string(digest), "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "SHA2-256=")
		if !ok {
			continue
		}
		value = strings.ToLower(strings.TrimSpace(value))
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != sha256.Size {
			return "", fmt.Errorf("xray checksum: malformed SHA2-256 entry in digest")
		}
		return value, nil
	}
	return "", fmt.Errorf("xray checksum: no SHA2-256 entry in digest")
}

func stageXrayBinary(archivePath, archiveBinaryName, targetPath string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	var source *zip.File
	for _, file := range reader.File {
		if file.Name == archiveBinaryName {
			source = file
			break
		}
	}
	if source == nil {
		return "", fmt.Errorf("xray archive does not contain %q", archiveBinaryName)
	}
	if source.UncompressedSize64 > maxXrayBinaryBytes {
		return "", fmt.Errorf("xray binary exceeds %d bytes", maxXrayBinaryBytes)
	}

	input, err := source.Open()
	if err != nil {
		return "", err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", err
	}
	output, err := os.CreateTemp(filepath.Dir(targetPath), ".xray-*")
	if err != nil {
		return "", err
	}
	stagedPath := output.Name()
	complete := false
	defer func() {
		_ = output.Close()
		if !complete {
			_ = os.Remove(stagedPath)
		}
	}()

	n, err := io.Copy(output, io.LimitReader(input, maxXrayBinaryBytes+1))
	if err != nil {
		return "", err
	}
	if n > maxXrayBinaryBytes {
		return "", fmt.Errorf("xray binary exceeds %d bytes", maxXrayBinaryBytes)
	}
	if err := output.Chmod(0o755); err != nil {
		return "", err
	}
	if err := output.Close(); err != nil {
		return "", err
	}
	complete = true
	return stagedPath, nil
}

func (s *ServerService) UpdateXray(version string) error {
	versions, err := s.GetXrayVersions()
	if err != nil {
		return err
	}
	if !slices.Contains(versions, version) {
		return fmt.Errorf("xray version %q is not in the fetched release list", version)
	}

	archivePath, err := s.downloadXray(version)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	_, archiveBinaryName, err := xrayReleaseAsset()
	if err != nil {
		return err
	}
	targetPath := xray.GetBinaryPath()
	stagedPath, err := stageXrayBinary(archivePath, archiveBinaryName, targetPath)
	if err != nil {
		return err
	}
	defer os.Remove(stagedPath)

	if err := s.StopXrayService(); err != nil {
		return err
	}
	restartNeeded := true
	defer func() {
		if restartNeeded {
			if restartErr := s.RestartXrayService(); restartErr != nil {
				logger.Warning("failed to restart xray after update error:", restartErr)
			}
		}
	}()

	if err := os.Rename(stagedPath, targetPath); err != nil {
		return err
	}
	if err := s.RestartXrayService(); err != nil {
		return err
	}
	restartNeeded = false
	return nil
}
