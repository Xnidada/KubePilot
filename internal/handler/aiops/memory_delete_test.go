package aiops

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestForgetMemoryRejectsInvalidSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := &Handler{}
	router.DELETE("/memories/:id", h.ForgetMemory)
	router.POST("/memories/batch-forget", h.BatchForgetMemories)

	for _, id := range []string{"0", "abc", "4294967296"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/memories/"+id, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("id %q: got %d", id, recorder.Code)
		}
	}
	for _, body := range []string{`{"ids":[]}`, `{"ids":[0]}`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/memories/batch-forget", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("body %s: got %d", body, recorder.Code)
		}
	}
}
