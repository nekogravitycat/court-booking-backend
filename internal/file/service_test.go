package file_test

import (
	"bytes"
	"context"
	"errors"
	"github.com/nekogravitycat/court-booking-backend/internal/file"
	"github.com/nekogravitycat/court-booking-backend/internal/pkg/storage"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
)

var databaseFailure = errors.New("database failed")

type failingFileRepo struct{ file.Repository }

func (failingFileRepo) Create(context.Context, *file.File) error {
	return databaseFailure
}

type successfulStorage struct {
	storage.Storage
	cleanupError error
}

func (successfulStorage) Save(context.Context, string, io.Reader) error { return nil }
func (s successfulStorage) Delete(context.Context, string) error        { return s.cleanupError }

// Cleanup must preserve the original database error.
func TestUploadPreservesDatabaseError(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="probe.txt"`},
		"Content-Type":        {"text/plain; charset=utf-8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "review probe"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(1024); err != nil {
		t.Fatal(err)
	}
	defer req.MultipartForm.RemoveAll()
	for _, cleanupErr := range []error{nil, errors.New("cleanup failed")} {
		service := file.NewService(failingFileRepo{}, successfulStorage{cleanupError: cleanupErr})
		f, err := service.Upload(context.Background(), file.UploadInput{FileHeader: req.MultipartForm.File["file"][0], MaxSizeBytes: 1024})
		if f != nil || !errors.Is(err, databaseFailure) {
			t.Fatalf("file=%v error=%v", f, err)
		}
	}
}
