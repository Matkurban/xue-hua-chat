package chat

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	chatmw "chat/internal/api/mw"
	"chat/pkg/common/errs"
	"github.com/gin-gonic/gin"
)

func TestListActiveConferencesRejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mw := chatmw.New(nil)
	r.POST("/conference/list_active", mw.CheckToken, (&Api{}).ListActiveConferences)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/conference/list_active", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("http status %d body %s", w.Code, w.Body.String())
	}
	var resp struct {
		ErrCode int    `json:"errCode"`
		ErrMsg  string `json:"errMsg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %v body=%s", err, w.Body.String())
	}
	if resp.ErrCode != errs.ArgsError {
		t.Fatalf("errCode=%d errMsg=%s body=%s", resp.ErrCode, resp.ErrMsg, w.Body.String())
	}
}
