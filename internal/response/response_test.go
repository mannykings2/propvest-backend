package response

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSuccessCarriesData guards against the variadic foot-gun where
// `Success(c, http.StatusOK, result, nil)` matched `case 3` and set
// `data = args[2] = nil`, silently dropping `result` and returning
// `{"success":true}` with no data. That bug emptied the payload of 15
// admin endpoints (dashboard, financial, users).
func TestSuccessCarriesData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		call     func(c *gin.Context)
		wantData bool
	}{
		{"2-arg (data)", func(c *gin.Context) {
			Success(c, map[string]interface{}{"a": 1})
		}, true},
		{"3-arg (code, data)", func(c *gin.Context) {
			Success(c, 200, map[string]interface{}{"a": 1})
		}, true},
		{"3-arg (code, message, data)", func(c *gin.Context) {
			Success(c, 200, "ok", map[string]interface{}{"a": 1})
		}, true},
		{"explicit Created (code, data)", func(c *gin.Context) {
			Success(c, 201, map[string]interface{}{"a": 1})
		}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/", nil)

			tc.call(c)

			var parsed struct {
				Success bool        `json:"success"`
				Data    interface{} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
				t.Fatalf("invalid JSON: %v (body=%s)", err, w.Body.String())
			}
			if !parsed.Success {
				t.Errorf("success = false, want true")
			}
			if tc.wantData && parsed.Data == nil {
				t.Errorf("data was dropped (got %s); want payload present", w.Body.String())
			}
		})
	}
}
