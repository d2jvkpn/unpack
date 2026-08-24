package unpack

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIsRemoteURL(t *testing.T) {
	tests := []struct {
		candidate string
		want      bool
	}{
		{"http://example.com/a.zip", true},
		{"https://example.com/a.zip", true},
		{"/local/path/a.zip", false},
		{"a.zip", false},
		{"ftp://example.com/a.zip", false},
	}
	for _, tt := range tests {
		t.Run(tt.candidate, func(t *testing.T) {
			if got := isRemoteURL(tt.candidate); got != tt.want {
				t.Fatalf("isRemoteURL(%q) = %v, want %v", tt.candidate, got, tt.want)
			}
		})
	}
}

func TestFetchArchiveFollowsRedirectAndUsesInputFilename(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start/original.zip", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final/archive.zip", http.StatusFound)
	})
	mux.HandleFunc("/final/archive.zip", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("payload")); err != nil {
			t.Fatal(err)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := newDownloadClient(5*time.Second, 5*time.Second, 5)

	localPath, cleanup, err := fetchArchive(server.URL+"/start/original.zip", client)
	if err != nil {
		t.Fatalf("fetchArchive() error = %v", err)
	}
	defer cleanup()

	if got := filepath.Base(localPath); got != "original.zip" {
		t.Fatalf("filepath.Base(localPath) = %q, want %q", got, "original.zip")
	}
	contents, err := os.ReadFile(localPath)
	if err != nil || string(contents) != "payload" {
		t.Fatalf("contents of %q = %q, %v; want %q", localPath, contents, err, "payload")
	}

	cleanup()
	if _, err := os.Stat(filepath.Dir(localPath)); !os.IsNotExist(err) {
		t.Fatalf("cleanup() did not remove temporary directory: %v", err)
	}
}

func TestFetchArchiveRejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := newDownloadClient(5*time.Second, 5*time.Second, 5)

	_, _, err := fetchArchive(server.URL+"/missing.zip", client)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("fetchArchive() error = %v, want error mentioning 404", err)
	}
}

func TestFetchArchiveRequiresInferableFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("payload")); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	client := newDownloadClient(5*time.Second, 5*time.Second, 5)

	_, _, err := fetchArchive(server.URL, client)
	if err == nil || !strings.Contains(err.Error(), "cannot determine archive filename") {
		t.Fatalf("fetchArchive() error = %v, want filename-inference error", err)
	}
}

func TestFetchArchiveStopsAfterMaxRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer server.Close()
	client := newDownloadClient(5*time.Second, 5*time.Second, 2)

	_, _, err := fetchArchive(server.URL+"/loop", client)
	if err == nil || !strings.Contains(err.Error(), "stopped after 2 redirects") {
		t.Fatalf("fetchArchive() error = %v, want redirect-limit error", err)
	}
}

func TestFetchArchiveRespectsHeaderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		if _, err := w.Write([]byte("payload")); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	client := newDownloadClient(20*time.Millisecond, 20*time.Millisecond, 5)

	_, _, err := fetchArchive(server.URL+"/slow.zip", client)
	if err == nil || !strings.Contains(err.Error(), "Client.Timeout") {
		t.Fatalf("fetchArchive() error = %v, want timeout error", err)
	}
}

func TestNewDownloadClientDefaults(t *testing.T) {
	client := newDownloadClient(downloadTimeout, downloadHeaderTimeout, downloadMaxRedirects)

	if client.Timeout != 5*time.Minute {
		t.Fatalf("client.Timeout = %v, want 5m", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client.Transport = %T, want *http.Transport", client.Transport)
	}
	if transport.ResponseHeaderTimeout != 10*time.Second {
		t.Fatalf("transport.ResponseHeaderTimeout = %v, want 10s", transport.ResponseHeaderTimeout)
	}

	via := make([]*http.Request, 5)
	if err := client.CheckRedirect(nil, via); err == nil {
		t.Fatal("CheckRedirect(5 prior requests) = nil, want error at the 5-redirect limit")
	}
	if err := client.CheckRedirect(nil, via[:4]); err != nil {
		t.Fatalf("CheckRedirect(4 prior requests) = %v, want nil", err)
	}
}

func TestNewDownloadClientUsesEnvironmentProxy(t *testing.T) {
	client := newDownloadClient(downloadTimeout, downloadHeaderTimeout, downloadMaxRedirects)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client.Transport = %T, want *http.Transport", client.Transport)
	}
	got := reflect.ValueOf(transport.Proxy).Pointer()
	want := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
	if got != want {
		t.Fatal("transport.Proxy is not http.ProxyFromEnvironment; " +
			"http_proxy/https_proxy/no_proxy would be ignored")
	}
}
