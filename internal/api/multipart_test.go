package api

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallMultipart_SendsFieldsAndFile(t *testing.T) {
	var gotField, gotFilename string
	var gotFileBytes []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if !strings.HasPrefix(mediaType, "multipart/") {
			t.Errorf("not multipart: %q", mediaType)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(p)
			switch p.FormName() {
			case "reason":
				gotField = string(data)
			case "image":
				gotFilename = p.FileName()
				gotFileBytes = data
			}
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New("xoxp-test")
	c.BaseURL = srv.URL
	_, err := c.CallMultipart("users.setPhoto",
		map[string]string{"reason": "update"}, "image", "avatar.png", []byte("PNGDATA"))
	if err != nil {
		t.Fatalf("multipart call: %v", err)
	}
	if gotField != "update" {
		t.Errorf("field not sent: %q", gotField)
	}
	if gotFilename != "avatar.png" {
		t.Errorf("filename not sent: %q", gotFilename)
	}
	if string(gotFileBytes) != "PNGDATA" {
		t.Errorf("file bytes not sent: %q", gotFileBytes)
	}
}
