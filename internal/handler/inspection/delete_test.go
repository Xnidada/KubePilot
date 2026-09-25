package inspection

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDeleteReportRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/reports/:id", (&InspectionHandler{}).DeleteReport)

	for _, id := range []string{"0", "abc", "4294967296"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/reports/"+id, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("id %q: expected 400, got %d", id, recorder.Code)
		}
	}
}

func TestBatchDeleteReportsRejectsInvalidSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/reports/batch-delete", (&InspectionHandler{}).BatchDeleteReports)

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
		request := httptest.NewRequest(http.MethodPost, "/reports/batch-delete", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("ids %v: expected 400, got %d", ids, recorder.Code)
		}
	}
}

func TestUniqueInspectionReportIDs(t *testing.T) {
	if got := uniqueInspectionReportIDs([]uint{5, 2, 5, 3, 2}); !reflect.DeepEqual(got, []uint{5, 2, 3}) {
		t.Fatalf("unexpected unique IDs: %v", got)
	}
}
