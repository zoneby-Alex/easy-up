package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func setupTestServer(t *testing.T) (*mux.Router, string) {
	t.Helper()

	uploadDir := t.TempDir()
	config = Config{
		UploadDir:       uploadDir,
		MaxFileSize:     8,
		AllowDeletes:    true,
		Users:           []UserConfig{{Username: "admin", Password: "secret"}},
		SessionTTLHours: 1,
	}
	sessionSecret = []byte("test-secret")

	r := mux.NewRouter()
	r.HandleFunc("/api/login", handleLogin).Methods("POST")
	r.HandleFunc("/api/logout", handleLogout).Methods("POST")
	r.HandleFunc("/api/session", handleSession).Methods("GET")
	r.HandleFunc("/api/upload", requireAuth(handleUpload)).Methods("POST")
	r.HandleFunc("/api/files", requireAuth(handleFilesList)).Methods("GET")
	r.HandleFunc("/api/create_folder", requireAuth(handleCreateFolder)).Methods("POST")
	r.HandleFunc("/api/delete/{filename}", requireAuth(handleDelete)).Methods("DELETE")
	r.PathPrefix("/download/").HandlerFunc(requireAuth(handleDownload)).Methods("GET")

	return r, uploadDir
}

func loginCookie(t *testing.T, r http.Handler) *http.Cookie {
	t.Helper()

	body := strings.NewReader(`{"username":"admin","password":"secret"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/login", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rr.Code, rr.Body.String())
	}
	for _, cookie := range rr.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatal("session cookie not found")
	return nil
}

func multipartUploadRequest(t *testing.T, target, fieldName, filename string, content []byte) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestProtectedEndpointsRequireLogin(t *testing.T) {
	r, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestLoginRejectsBadPassword(t *testing.T) {
	r, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"admin","password":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestUploadRejectsOversizedFile(t *testing.T) {
	r, uploadDir := setupTestServer(t)
	cookie := loginCookie(t, r)

	req := multipartUploadRequest(t, "/api/upload", "file", "large.jpg", []byte("123456789"))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, http.StatusRequestEntityTooLarge, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(uploadDir, "large.jpg")); !os.IsNotExist(err) {
		t.Fatalf("oversized file should not exist, stat err = %v", err)
	}
}

func TestUploadAutoRenamesDuplicateFile(t *testing.T) {
	r, uploadDir := setupTestServer(t)
	cookie := loginCookie(t, r)

	if err := os.WriteFile(filepath.Join(uploadDir, "photo.jpg"), []byte("old"), 0666); err != nil {
		t.Fatal(err)
	}

	req := multipartUploadRequest(t, "/api/upload", "file", "photo.jpg", []byte("new"))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["name"] != "photo-1.jpg" {
		t.Fatalf("name = %v, want photo-1.jpg", resp["name"])
	}

	oldContent, err := os.ReadFile(filepath.Join(uploadDir, "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(oldContent) != "old" {
		t.Fatalf("old file was overwritten: %q", oldContent)
	}
}

func TestUploadRejectsSVG(t *testing.T) {
	r, _ := setupTestServer(t)
	cookie := loginCookie(t, r)

	req := multipartUploadRequest(t, "/api/upload", "file", "icon.svg", []byte("<svg></svg>"))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUploadAllowsOfficeCodeAndArchiveFiles(t *testing.T) {
	r, uploadDir := setupTestServer(t)
	cookie := loginCookie(t, r)

	for _, filename := range []string{"deck.pptx", "sheet.xlsx", "doc.docx", "notes.md", "main.go", "report.pdf", "archive.zip", "bundle.7z", "backup.tar", "logs.gz"} {
		req := multipartUploadRequest(t, "/api/upload", "file", filename, []byte("ok"))
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("%s upload status = %d, want %d, body = %s", filename, rr.Code, http.StatusOK, rr.Body.String())
		}
		if _, err := os.Stat(filepath.Join(uploadDir, filename)); err != nil {
			t.Fatalf("%s should exist: %v", filename, err)
		}
	}
}

func TestDeleteEmptyDirectoryOnly(t *testing.T) {
	r, uploadDir := setupTestServer(t)
	cookie := loginCookie(t, r)

	if err := os.Mkdir(filepath.Join(uploadDir, "empty"), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(uploadDir, "full"), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "full", "file.jpg"), []byte("x"), 0666); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/delete/empty", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty dir delete status = %d, body = %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/delete/full", nil)
	req.AddCookie(cookie)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("non-empty dir delete status = %d, want %d", rr.Code, http.StatusConflict)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, "full", "file.jpg")); err != nil {
		t.Fatalf("non-empty directory contents should remain: %v", err)
	}
}
