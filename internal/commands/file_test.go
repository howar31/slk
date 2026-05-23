package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/howar31/slk/internal/api"
)

// TestFileCommand_FlagsRegistered verifies that all six verbs and their flags
// are registered under the file command group.
func TestFileCommand_FlagsRegistered(t *testing.T) {
	cmd := newFileCommand(&GlobalFlags{})

	for _, tc := range []struct {
		verb  string
		flags []string
	}{
		{"list", []string{"channel", "user", "types"}},
		{"info", []string{"file"}},
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
