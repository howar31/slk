package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/howar31/slk/internal/api"
	"github.com/spf13/cobra"
)

// TestFileCommand_FlagsRegistered verifies that all verbs and their flags
// are registered under the file command group.
func TestFileCommand_FlagsRegistered(t *testing.T) {
	cmd := newFileCommand(&GlobalFlags{})

	for _, tc := range []struct {
		verb  string
		flags []string
	}{
		{"list", []string{"channel", "user", "types"}},
		{"info", []string{"file"}},
		{"download", []string{"file", "out", "force"}},
		{"upload", []string{"file", "channel", "title"}},
		{"delete", []string{"file"}},
		{"public", []string{"file"}},
		{"revoke-public", []string{"file"}},
	} {
		sub, _, err := cmd.Find([]string{tc.verb})
		if err != nil {
			t.Fatalf("find %s: %v", tc.verb, err)
		}
		for _, flag := range tc.flags {
			if sub.Flags().Lookup(flag) == nil {
				t.Errorf("missing --%s on file %s", flag, tc.verb)
			}
		}
	}
}

// TestFileWrite_DryRun verifies that write verbs print the expected dry-run
// line and make no HTTP calls.
func TestFileWrite_DryRun(t *testing.T) {
	// Create a temporary file for the upload dry-run test.
	tmpFile, err := os.CreateTemp(t.TempDir(), "slk-test-*.txt")
	if err != nil {
		t.Fatalf("create tmp file: %v", err)
	}
	_, _ = tmpFile.WriteString("hello")
	tmpFile.Close()

	for _, tc := range []struct {
		args []string
		want string
	}{
		{
			args: []string{"upload", "--file", tmpFile.Name()},
			want: "files.upload",
		},
		{
			args: []string{"delete", "--file", "F01234567"},
			want: "files.delete",
		},
		{
			args: []string{"public", "--file", "F01234567"},
			want: "files.sharedPublicURL",
		},
		{
			args: []string{"revoke-public", "--file", "F01234567"},
			want: "files.revokePublicURL",
		},
	} {
		g := &GlobalFlags{DryRun: true}
		cmd := newFileCommand(g)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Fatalf("want %q in output %q for args %v", tc.want, out.String(), tc.args)
		}
	}
}

// TestParseFileInfo verifies that parseFileInfo correctly extracts fields from
// a files.info response.
func TestParseFileInfo(t *testing.T) {
	raw := []byte(`{
		"ok": true,
		"file": {
			"id": "F01234567",
			"name": "report.pdf",
			"filetype": "pdf"
		}
	}`)
	hit, err := parseFileInfo(raw)
	if err != nil {
		t.Fatalf("parseFileInfo: %v", err)
	}
	if hit.ID != "F01234567" {
		t.Errorf("ID: got %q, want F01234567", hit.ID)
	}
	if hit.Name != "report.pdf" {
		t.Errorf("Name: got %q, want report.pdf", hit.Name)
	}
	if hit.Extra != "pdf" {
		t.Errorf("Extra: got %q, want pdf", hit.Extra)
	}
}

// TestFileFetchList_HTTPTest verifies that fileFetchList pages correctly and
// stops at the last page.
func TestFileFetchList_HTTPTest(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		calls++
		page := r.Form.Get("page")

		var totalPages int
		var files []map[string]any
		switch page {
		case "1":
			totalPages = 2
			files = []map[string]any{
				{"id": "F01234567", "name": "alice.png", "filetype": "png"},
			}
		default:
			totalPages = 2
			files = []map[string]any{
				{"id": "F01234568", "name": "bob.txt", "filetype": "text"},
			}
		}

		resp := map[string]any{
			"ok":    true,
			"files": files,
			"paging": map[string]any{
				"page":  calls,
				"pages": totalPages,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	hits, err := fileFetchList(c, "", "", "")
	if err != nil {
		t.Fatalf("fileFetchList: %v", err)
	}
	if len(hits) != 2 {
		t.Errorf("got %d hits, want 2", len(hits))
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", calls)
	}
	if hits[0].ID != "F01234567" {
		t.Errorf("hits[0].ID: got %q, want F01234567", hits[0].ID)
	}
	if hits[1].Name != "bob.txt" {
		t.Errorf("hits[1].Name: got %q, want bob.txt", hits[1].Name)
	}
}

// TestFileInfo_HTTPTest verifies the info command round-trips through the parse
// helper correctly.
func TestFileInfo_HTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		resp := map[string]any{
			"ok": true,
			"file": map[string]any{
				"id":       "F01234567",
				"name":     "diagram.png",
				"filetype": "png",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	raw, err := c.Call("files.info", map[string]string{"file": "F01234567"}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	hit, err := parseFileInfo(raw)
	if err != nil {
		t.Fatalf("parseFileInfo: %v", err)
	}
	if hit.ID != "F01234567" {
		t.Errorf("ID: got %q, want F01234567", hit.ID)
	}
	if hit.Name != "diagram.png" {
		t.Errorf("Name: got %q, want diagram.png", hit.Name)
	}
	if hit.Extra != "png" {
		t.Errorf("Extra: got %q, want png", hit.Extra)
	}
}

// TestFileUpload_HTTPTest exercises the three-step upload flow:
// (1) files.getUploadURLExternal, (2) raw PUT to the upload URL,
// (3) files.completeUploadExternal.
func TestFileUpload_HTTPTest(t *testing.T) {
	// Track which endpoints were hit.
	var hitGetURL, hitUpload, hitComplete bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files.getUploadURLExternal"):
			hitGetURL = true
			_ = r.ParseForm()
			resp := map[string]any{
				"ok":         true,
				"upload_url": "http://" + r.Host + "/upload",
				"file_id":    "F01234567",
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/upload":
			hitUpload = true
			body, _ := io.ReadAll(r.Body)
			if string(body) != "hello slk" {
				t.Errorf("upload body: got %q, want %q", string(body), "hello slk")
			}
			w.WriteHeader(http.StatusOK)

		case strings.HasSuffix(r.URL.Path, "/files.completeUploadExternal"):
			hitComplete = true
			_ = r.ParseForm()
			filesParam := r.Form.Get("files")
			if !strings.Contains(filesParam, "F01234567") {
				t.Errorf("completeUploadExternal missing file_id in files param: %q", filesParam)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}
	}))
	defer srv.Close()

	// Write a temporary file with known content.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello slk"), 0600); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}

	// Run the upload command against the test server via the upload logic
	// directly (using a fake client that points at srv).
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL

	// Step 1: get upload URL.
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	raw, err := c.Call("files.getUploadURLExternal", map[string]string{
		"filename": filepath.Base(filePath),
		"length":   fmt.Sprint(len(data)),
	}, nil)
	if err != nil {
		t.Fatalf("getUploadURLExternal: %v", err)
	}
	var urlResp struct {
		UploadURL string `json:"upload_url"`
		FileID    string `json:"file_id"`
	}
	if err := json.Unmarshal(raw, &urlResp); err != nil {
		t.Fatalf("unmarshal url resp: %v", err)
	}

	// Step 2: raw POST to the upload URL.
	uploadResp, err := http.Post(urlResp.UploadURL, "application/octet-stream", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("upload POST: %v", err)
	}
	uploadResp.Body.Close()
	if uploadResp.StatusCode < 200 || uploadResp.StatusCode >= 300 {
		t.Fatalf("upload status: %d", uploadResp.StatusCode)
	}

	// Step 3: complete the upload.
	filesJSON := fmt.Sprintf(`[{"id":%q,"title":%q}]`, urlResp.FileID, filepath.Base(filePath))
	_, err = c.Call("files.completeUploadExternal", map[string]string{"files": filesJSON}, nil)
	if err != nil {
		t.Fatalf("completeUploadExternal: %v", err)
	}

	if !hitGetURL {
		t.Error("files.getUploadURLExternal was not called")
	}
	if !hitUpload {
		t.Error("upload POST was not called")
	}
	if !hitComplete {
		t.Error("files.completeUploadExternal was not called")
	}
	if urlResp.FileID != "F01234567" {
		t.Errorf("FileID: got %q, want F01234567", urlResp.FileID)
	}
}

// fakeSlackFile is one file served by newFakeSlackFiles.
type fakeSlackFile struct {
	name, mimetype, contentType, body string
}

// fakeSlackFiles answers files.info and serves file bytes from one TLS server.
// fileHits counts GETs per file ID; infoHits counts files.info calls.
type fakeSlackFiles struct {
	client   *api.Client
	mu       sync.Mutex
	fileHits map[string]int
	infoHits int
}

func (f *fakeSlackFiles) hits(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fileHits[id]
}

func (f *fakeSlackFiles) infoCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.infoHits
}

func newFakeSlackFiles(t *testing.T, files map[string]fakeSlackFile) *fakeSlackFiles {
	t.Helper()
	fake := &fakeSlackFiles{fileHits: map[string]int{}}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/files.info" {
			fake.mu.Lock()
			fake.infoHits++
			fake.mu.Unlock()
			_ = r.ParseForm()
			id := r.Form.Get("file")
			file, ok := files[id]
			if !ok {
				fmt.Fprint(w, `{"ok":false,"error":"file_not_found"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "file": map[string]any{
				"id":          id,
				"name":        file.name,
				"mimetype":    file.mimetype,
				"size":        len(file.body),
				"mode":        "hosted",
				"url_private": "https://" + r.Host + "/files-pri/" + id,
			}})
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/files-pri/")
		fake.mu.Lock()
		fake.fileHits[id]++
		fake.mu.Unlock()
		w.Header().Set("Content-Type", files[id].contentType)
		fmt.Fprint(w, files[id].body)
	}))
	t.Cleanup(srv.Close)
	c := api.New("xoxp-test")
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()
	c.FileHosts = []string{"127.0.0.1"}
	fake.client = c
	return fake
}

func TestSaveFile_WritesOwnerOnlyFile(t *testing.T) {
	fake := newFakeSlackFiles(t, map[string]fakeSlackFile{
		"F01234567": {"diagram.png", "image/png", "image/png", "png-bytes"},
	})
	info, err := fetchFileMeta(fake.client, "F01234567")
	if err != nil {
		t.Fatalf("fetchFileMeta: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "diagram.png")

	if err := saveFile(fake.client, info, dest); err != nil {
		t.Fatalf("saveFile: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(got) != "png-bytes" {
		t.Errorf("content = %q, want png-bytes", got)
	}
	stat, _ := os.Stat(dest)
	if stat.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", stat.Mode().Perm())
	}
}

func TestSaveFile_LoginPageForImageLeavesNothingBehind(t *testing.T) {
	fake := newFakeSlackFiles(t, map[string]fakeSlackFile{
		"F01234567": {"diagram.png", "image/png", "text/html; charset=utf-8", "<html>sign in</html>"},
	})
	info, err := fetchFileMeta(fake.client, "F01234567")
	if err != nil {
		t.Fatalf("fetchFileMeta: %v", err)
	}
	dir := t.TempDir()

	err = saveFile(fake.client, info, filepath.Join(dir, "diagram.png"))
	if err == nil {
		t.Fatal("expected an error for an HTML response to an image")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("left %d entries in the directory, want 0", len(entries))
	}
}

func TestSaveFile_TruncatedDownloadLeavesNothingBehind(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, "short")
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.HTTP = srv.Client()
	c.FileHosts = []string{"127.0.0.1"}
	dir := t.TempDir()

	err := saveFile(c, slackFile{ID: "F01234567", URLPrivate: srv.URL + "/image.png"}, filepath.Join(dir, "image.png"))
	if err == nil {
		t.Fatal("expected an error for a truncated download")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("left %d entries in the directory, want 0", len(entries))
	}
}

func TestSaveFile_HTMLFileIsSaved(t *testing.T) {
	fake := newFakeSlackFiles(t, map[string]fakeSlackFile{
		"F01234567": {"page.html", "text/html", "text/html", "<html>page</html>"},
	})
	info, err := fetchFileMeta(fake.client, "F01234567")
	if err != nil {
		t.Fatalf("fetchFileMeta: %v", err)
	}
	if err := saveFile(fake.client, info, filepath.Join(t.TempDir(), "page.html")); err != nil {
		t.Fatalf("saveFile: %v", err)
	}
}

func TestSaveFile_RemoteFileSendsNoToken(t *testing.T) {
	var hits atomic.Int32
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer remote.Close()
	c := api.New("xoxp-test")
	c.HTTP = remote.Client()
	info := slackFile{ID: "F01234567", Mimetype: "image/png", URLPrivate: remote.URL + "/image.png"}

	if err := saveFile(c, info, filepath.Join(t.TempDir(), "image.png")); err == nil {
		t.Fatal("expected an error for a remote file")
	}
	if hits.Load() != 0 {
		t.Errorf("remote host received %d requests, want 0", hits.Load())
	}
}

func TestSaveFile_PrefersDownloadURL(t *testing.T) {
	var gotPath atomic.Value
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath.Store(r.URL.Path)
	}))
	defer srv.Close()
	c := api.New("xoxp-test")
	c.HTTP = srv.Client()
	c.FileHosts = []string{"127.0.0.1"}
	info := slackFile{URLPrivate: srv.URL + "/view", URLPrivateDownload: srv.URL + "/download"}

	if err := saveFile(c, info, filepath.Join(t.TempDir(), "a.bin")); err != nil {
		t.Fatalf("saveFile: %v", err)
	}
	if gotPath.Load() != "/download" {
		t.Errorf("fetched %v, want /download", gotPath.Load())
	}
}

func TestSaveFile_NoURLIsAnError(t *testing.T) {
	c := api.New("xoxp-test")
	if err := saveFile(c, slackFile{ID: "F01234567"}, filepath.Join(t.TempDir(), "a.bin")); err == nil {
		t.Fatal("expected an error for a file without a URL")
	}
}

func TestSafeFileName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"image.png", "image.png"},
		{"我的 截圖.png", "我的 截圖.png"},
		{"../../etc/passwd", "passwd"},
		{`..\..\windows\win.ini`, "win.ini"},
		{"/etc/passwd", "passwd"},
		{".bashrc", "bashrc"},
		{"..", ""},
		{"", ""},
		{"a:b*c?.png", "a_b_c_.png"},
		{"line\nbreak.png", "line_break.png"},
	}
	for _, tc := range cases {
		if got := safeFileName(tc.in); got != tc.want {
			t.Errorf("safeFileName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDownloadDest(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.png")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	cases := []struct {
		name, out, fileName string
		force, wantErr      bool
		want                string
	}{
		{name: "defaults to the file name", fileName: "new.png", want: "new.png"},
		{name: "strips path from the file name", fileName: "../up.png", want: "up.png"},
		{name: "explicit path", out: filepath.Join(dir, "x.png"), fileName: "new.png", want: filepath.Join(dir, "x.png")},
		{name: "directory gets the file name", out: dir, fileName: "new.png", want: filepath.Join(dir, "new.png")},
		{name: "existing file is refused", out: existing, fileName: "new.png", wantErr: true},
		{name: "existing file is refused via directory", out: dir, fileName: "existing.png", wantErr: true},
		{name: "force allows overwrite", out: existing, fileName: "new.png", force: true, want: existing},
		{name: "unusable name without --out", fileName: "..", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := downloadDest(tc.out, tc.fileName, tc.force)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("downloadDest = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("downloadDest: %v", err)
			}
			if got != tc.want {
				t.Errorf("downloadDest = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFileDownload_DryRunMakesNoCall(t *testing.T) {
	g := &GlobalFlags{DryRun: true}
	cmd := newFileCommand(g)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"download", "--file", "F01234567"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "files.info") || !strings.Contains(out.String(), "F01234567") {
		t.Errorf("dry-run output = %q", out.String())
	}
}

func TestSaveMessageImages_SameNameFilesStaySeparate(t *testing.T) {
	fake := newFakeSlackFiles(t, map[string]fakeSlackFile{
		"F0000000A": {"image.png", "image/png", "image/png", "first"},
		"F0000000B": {"image.png", "image/png", "image/png", "second"},
	})
	items := []msgItem{
		{Files: []msgFile{{ID: "F0000000A", Name: "image.png", Mimetype: "image/png"}}},
		{Files: []msgFile{{ID: "F0000000B", Name: "image.png", Mimetype: "image/png"}}},
	}

	if failed := saveMessageImages(fake.client, items, t.TempDir()); failed != 0 {
		t.Fatalf("failed = %d, items = %+v", failed, items)
	}

	first, _ := os.ReadFile(items[0].Files[0].Path)
	second, _ := os.ReadFile(items[1].Files[0].Path)
	if string(first) != "first" || string(second) != "second" {
		t.Errorf("contents = %q, %q", first, second)
	}
	if !filepath.IsAbs(items[0].Files[0].Path) {
		t.Errorf("path %q is not absolute", items[0].Files[0].Path)
	}
}

func TestSaveMessageImages_SkipsNonImages(t *testing.T) {
	fake := newFakeSlackFiles(t, map[string]fakeSlackFile{
		"F0000000A": {"report.pdf", "application/pdf", "application/pdf", "pdf"},
	})
	items := []msgItem{{Files: []msgFile{{ID: "F0000000A", Name: "report.pdf", Mimetype: "application/pdf"}}}}

	if failed := saveMessageImages(fake.client, items, t.TempDir()); failed != 0 {
		t.Fatalf("failed = %d", failed)
	}
	if fake.hits("F0000000A") != 0 || fake.infoCalls() != 0 {
		t.Errorf("non-image touched Slack: downloads=%d info=%d", fake.hits("F0000000A"), fake.infoCalls())
	}
	if items[0].Files[0].Path != "" {
		t.Errorf("path = %q, want empty", items[0].Files[0].Path)
	}
}

func TestSaveMessageImages_StubResolvedThroughFilesInfo(t *testing.T) {
	fake := newFakeSlackFiles(t, map[string]fakeSlackFile{
		"F0000000A": {"shared.png", "image/png", "image/png", "bytes"},
		"F0000000B": {"shared.pdf", "application/pdf", "application/pdf", "bytes"},
	})
	items := []msgItem{{Files: []msgFile{{ID: "F0000000A"}, {ID: "F0000000B"}}}}

	if failed := saveMessageImages(fake.client, items, t.TempDir()); failed != 0 {
		t.Fatalf("failed = %d, items = %+v", failed, items)
	}

	image, pdf := items[0].Files[0], items[0].Files[1]
	if image.Name != "shared.png" || image.Mimetype != "image/png" || image.Path == "" {
		t.Errorf("image = %+v", image)
	}
	if pdf.Name != "shared.pdf" || pdf.Path != "" || fake.hits("F0000000B") != 0 {
		t.Errorf("pdf = %+v, downloads = %d", pdf, fake.hits("F0000000B"))
	}
}

func TestSaveMessageImages_FailureIsRecordedAndOthersContinue(t *testing.T) {
	fake := newFakeSlackFiles(t, map[string]fakeSlackFile{
		"F0000000B": {"ok.png", "image/png", "image/png", "bytes"},
	})
	items := []msgItem{{Files: []msgFile{
		{ID: "F0000000A", Name: "gone.png", Mimetype: "image/png"},
		{ID: "F0000000B", Name: "ok.png", Mimetype: "image/png"},
	}}}

	if failed := saveMessageImages(fake.client, items, t.TempDir()); failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
	gone, ok := items[0].Files[0], items[0].Files[1]
	if !strings.Contains(gone.SaveError, "file_not_found") || gone.Path != "" {
		t.Errorf("gone = %+v", gone)
	}
	if ok.Path == "" {
		t.Errorf("ok = %+v", ok)
	}
}

func TestSaveMessageImages_RereadReusesSavedFile(t *testing.T) {
	fake := newFakeSlackFiles(t, map[string]fakeSlackFile{
		"F0000000A": {"image.png", "image/png", "image/png", "bytes"},
	})
	dir := t.TempDir()
	read := func() []msgItem {
		return []msgItem{{Files: []msgFile{{ID: "F0000000A", Name: "image.png", Mimetype: "image/png"}}}}
	}
	saveMessageImages(fake.client, read(), dir)

	items := read()
	if failed := saveMessageImages(fake.client, items, dir); failed != 0 {
		t.Fatalf("failed = %d", failed)
	}

	if fake.hits("F0000000A") != 1 || fake.infoCalls() != 1 {
		t.Errorf("downloads = %d, info calls = %d, want 1 and 1", fake.hits("F0000000A"), fake.infoCalls())
	}
	if items[0].Files[0].Path == "" {
		t.Error("second read did not report the saved path")
	}
}

// fakeSlack stands in for slack.com and files.slack.com at the transport level,
// so a test runs a whole command, with api.New's real defaults, on a fake token.
// Every request reaches handler with its original Host.
func fakeSlack(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	transport := srv.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.InsecureSkipVerify = true
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	original := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original })
	t.Setenv("SLK_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("SLK_TOKEN", "xoxp-test")
}

// fakeSlackHandler answers one message-read method with a message carrying
// F01234567, plus files.info and the file itself on files.slack.com. Unknown
// requests fail the test. A file request without the token gets a 403.
func fakeSlackHandler(t *testing.T, method string) http.HandlerFunc {
	const fileURL = "https://files.slack.com/files-pri/T0123456789-F01234567/image.png"
	file := map[string]any{
		"id": "F01234567", "name": "image.png", "mimetype": "image/png", "size": 9,
		"mode": "hosted", "url_private": fileURL,
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "slack.com" && r.URL.Path == "/api/"+method:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "messages": []map[string]any{
				{"user": "U0123456789", "text": "", "ts": "1700000000.123456", "files": []any{file}},
			}})
		case r.Host == "slack.com" && r.URL.Path == "/api/files.info":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "file": file})
		case r.Host == "files.slack.com" && r.Header.Get("Authorization") == "Bearer xoxp-test":
			w.Header().Set("Content-Type", "image/png")
			fmt.Fprint(w, "png-bytes")
		case r.Host == "files.slack.com":
			w.WriteHeader(http.StatusForbidden)
		default:
			t.Errorf("unexpected request: %s %s", r.Host, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// savedMessage runs a read command and returns its one message's one file.
func savedMessage(t *testing.T, cmd *cobra.Command, args ...string) msgFile {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var items []msgItem
	if err := json.Unmarshal(out.Bytes(), &items); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	if len(items) != 1 || len(items[0].Files) != 1 {
		t.Fatalf("items = %+v, want one message with one file", items)
	}
	return items[0].Files[0]
}

func TestMsgRead_ChannelListsAttachedFiles(t *testing.T) {
	fakeSlack(t, fakeSlackHandler(t, "conversations.history"))

	file := savedMessage(t, newMsgCommand(&GlobalFlags{Format: "json", NoResolve: true}),
		"read", "--channel", "C0123456789")

	if file.ID != "F01234567" || file.Mimetype != "image/png" || file.Path != "" {
		t.Errorf("file = %+v", file)
	}
}

func TestMsgRead_MessageURLSavesImage(t *testing.T) {
	fakeSlack(t, fakeSlackHandler(t, "conversations.history"))
	dir := t.TempDir()

	file := savedMessage(t, newMsgCommand(&GlobalFlags{Format: "json", NoResolve: true}),
		"read", "--url", "https://acme.slack.com/archives/C0123456789/p1700000000123456", "--save-images", dir)

	got, err := os.ReadFile(file.Path)
	if err != nil {
		t.Fatalf("read saved image: %v", err)
	}
	if string(got) != "png-bytes" {
		t.Errorf("content = %q", got)
	}
}

func TestThreadRead_MessageURLSavesImage(t *testing.T) {
	fakeSlack(t, fakeSlackHandler(t, "conversations.replies"))
	dir := t.TempDir()

	file := savedMessage(t, newThreadCommand(&GlobalFlags{Format: "json", NoResolve: true}),
		"read", "--url", "https://acme.slack.com/archives/C0123456789/p1700000000123456", "--save-images", dir)

	got, err := os.ReadFile(file.Path)
	if err != nil {
		t.Fatalf("read saved image: %v", err)
	}
	if string(got) != "png-bytes" {
		t.Errorf("content = %q", got)
	}
}

func TestMsgRead_UnsavableImageExitsNonZeroAfterOutput(t *testing.T) {
	handler := fakeSlackHandler(t, "conversations.history")
	fakeSlack(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "files.slack.com" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		handler(w, r)
	})
	cmd := newMsgCommand(&GlobalFlags{NoResolve: true})
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"read", "--channel", "C0123456789", "--save-images", t.TempDir()})

	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected a non-zero exit")
	}
	if !strings.Contains(out.String(), "F01234567") || !strings.Contains(out.String(), "save failed") {
		t.Errorf("output = %q, want the failure on the message line", out.String())
	}
}

func TestFileDownload_SavesToOut(t *testing.T) {
	fakeSlack(t, fakeSlackHandler(t, "conversations.history"))
	dest := filepath.Join(t.TempDir(), "saved.png")
	cmd := newFileCommand(&GlobalFlags{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"download", "--file", "F01234567", "--out", dest})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(got) != "png-bytes" {
		t.Errorf("content = %q", got)
	}
	if !strings.Contains(out.String(), dest) {
		t.Errorf("output = %q, want the saved path", out.String())
	}
}
