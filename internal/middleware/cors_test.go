package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSRequiresExplicitOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORSMiddleware([]string{"https://console.example.test"}))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })
	r.OPTIONS("/x", func(c *gin.Context) { c.Status(204) })
	for _, tc := range []struct {
		origin     string
		method     string
		wantCode   int
		wantOrigin string
	}{
		{"https://evil.example.test", http.MethodOptions, 403, ""},
		{"https://evil.example.test", http.MethodGet, 200, ""},
		{"https://console.example.test", http.MethodOptions, 204, "https://console.example.test"},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, "/x", nil)
		req.Header.Set("Origin", tc.origin)
		r.ServeHTTP(w, req)
		if w.Code != tc.wantCode || w.Header().Get("Access-Control-Allow-Origin") != tc.wantOrigin {
			t.Fatalf("%s %s: code=%d origin=%q", tc.method, tc.origin, w.Code, w.Header().Get("Access-Control-Allow-Origin"))
		}
	}
}
