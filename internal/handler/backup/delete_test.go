package backup

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDeleteBackupRecordRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/backups/:id", (&Handler{}).DeleteBackupRecord)

	for _, id := range []string{"0", "abc", "4294967296"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/backups/"+id, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("id %q: expected 400, got %d", id, recorder.Code)
		}
	}
}

func TestBatchDeleteBackupRecordsRejectsInvalidSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/backups/batch-delete", (&Handler{}).BatchDeleteBackupRecords)

	tooMany := make([]uint, 101)
	for i := range tooMany {
		tooMany[i] = uint(i + 1)
	}
	for _, ids := range [][]uint{nil, {}, {0}, {1, 0}, tooMany} {
		body, err := json.Marshal(map[string]interface{}{"ids": ids})
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/backups/batch-delete", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("ids %v: expected 400, got %d", ids, recorder.Code)
		}
	}
}

func TestBackupDeleteSelectionAndActiveStatus(t *testing.T) {
	if got := uniqueBackupIDs([]uint{5, 2, 5, 3, 2}); !reflect.DeepEqual(got, []uint{5, 2, 3}) {
		t.Fatalf("unexpected unique IDs: %v", got)
	}
	for status, active := range map[string]bool{
		"pending": true, "in_progress": true, "completed": false, "failed": false,
	} {
		if got := backupRecordActive(status); got != active {
			t.Errorf("status %q: got active=%v, want %v", status, got, active)
		}
	}
}

func TestDeleteRestoreRecordRejectsInvalidSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := &Handler{}
	router.DELETE("/backups/restores/:id", h.DeleteRestoreRecord)
	router.POST("/backups/restores/batch-delete", h.BatchDeleteRestoreRecords)

	for _, id := range []string{"0", "abc", "4294967296"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/backups/restores/"+id, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("id %q: expected 400, got %d", id, recorder.Code)
		}
	}
	tooMany := make([]uint, 101)
	for i := range tooMany {
		tooMany[i] = uint(i + 1)
	}
	for _, ids := range [][]uint{nil, {}, {0}, tooMany} {
		body, _ := json.Marshal(map[string]any{"ids": ids})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/backups/restores/batch-delete", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("ids %v: expected 400, got %d", ids, recorder.Code)
		}
	}
}
