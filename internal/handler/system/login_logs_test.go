package system

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDeleteLoginLogRejectsInvalidIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/login-logs/:id", (&Handler{}).DeleteLoginLog)

	for _, id := range []string{"0", "abc", "4294967296"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/login-logs/"+id, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("id %q: expected 400, got %d", id, recorder.Code)
		}
	}
}

func TestBatchDeleteLoginLogsRejectsInvalidSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/login-logs/batch-delete", (&Handler{}).BatchDeleteLoginLogs)

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
		request := httptest.NewRequest(http.MethodPost, "/login-logs/batch-delete", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("ids %v: expected 400, got %d", ids, recorder.Code)
		}
	}
}
