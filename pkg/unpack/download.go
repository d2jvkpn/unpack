package unpack

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	downloadTimeout       = 5 * time.Minute
	downloadHeaderTimeout = 10 * time.Second
	downloadMaxRedirects  = 5
)

// IsRemoteURL reports whether candidate looks like an http(s) URL rather than a local file path.
func IsRemoteURL(candidate string) bool {
	return strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://")
}

func newDownloadClient(timeout time.Duration, headerTimeout time.Duration, maxRedirects int) *http.Client {
	var transport *http.Transport

	transport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: headerTimeout,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("follow redirect: stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
}

// Download fetches rawURL into a temporary file and returns its local path and a cleanup
// function that removes the temporary file and its containing directory. The caller is
// responsible for calling cleanup once done with the file.
func Download(rawURL string) (string, func(), error) {
	return fetchArchive(rawURL, newDownloadClient(downloadTimeout, downloadHeaderTimeout, downloadMaxRedirects))
}

func fetchArchive(rawURL string, client *http.Client) (localPath string, cleanup func(), err error) {
	var (
		parsed   *url.URL
		filename string
		resp     *http.Response
		tempDir  string
		file     *os.File
	)

	parsed, err = url.Parse(rawURL)
	if err != nil {
		return "", nil, fmt.Errorf("download %q: %w", rawURL, err)
	}
	filename, err = downloadFilename(parsed)
	if err != nil {
		return "", nil, fmt.Errorf("download %q: %w", rawURL, err)
	}

	resp, err = client.Get(rawURL)
	if err != nil {
		return "", nil, fmt.Errorf("download %q: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("download %q: unexpected status %s", rawURL, resp.Status)
	}

	tempDir, err = os.MkdirTemp("", "unpack-download-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary directory: %w", err)
	}
	cleanup = func() { os.RemoveAll(tempDir) }

	localPath = filepath.Join(tempDir, filename)
	file, err = os.Create(localPath)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("create %q: %w", localPath, err)
	}

	if _, err = io.Copy(file, resp.Body); err != nil {
		file.Close()
		cleanup()
		return "", nil, fmt.Errorf("save %q: %w", rawURL, err)
	}
	if err = file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("save %q: %w", rawURL, err)
	}

	return localPath, cleanup, nil
}

func downloadFilename(source *url.URL) (string, error) {
	var name string

	name = path.Base(source.Path)
	if name == "." || name == "/" {
		return "", fmt.Errorf("cannot determine archive filename from URL %q", source)
	}
	return name, nil
}
