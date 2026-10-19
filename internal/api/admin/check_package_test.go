package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"chat/pkg/common/storage"
)

func TestCheckApplicationPackage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ps, _ := storage.NewPackageStorage(t.TempDir(), "/application/files", 0)
	sp, err := ps.SaveReader("Elephant-1.2.0.dmg", strings.NewReader("dmg-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	o := &Api{Packages: ps, PackageBaseURL: "https://im.example/chat-admin-api"}
	r := gin.New()
	r.POST("/application/check_package", o.CheckApplicationPackage)
	call := func(body string) string {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/application/check_package", strings.NewReader(body)))
		t.Log(body, "=>", w.Body.String())
		return w.Body.String()
	}
	found := call(`{"sha256":"` + strings.ToUpper(sp.Sha256) + `","size":9,"filename":"x.dmg"}`)
	if !strings.Contains(found, `"exists":true`) || !strings.Contains(found, `"url":"https://im.example/chat-admin-api`+sp.Path+`"`) || !strings.Contains(found, `"package_type":"dmg"`) {
		t.Fatal(found)
	}
	if s := call(`{"sha256":"` + sp.Sha256 + `","size":10}`); !strings.Contains(s, `"data":{"exists":false}`) {
		t.Fatal("size mismatch", s)
	}
	if s := call(`{"sha256":"zz","size":9}`); !strings.Contains(s, `"errCode":1001`) {
		t.Fatal("bad hash", s)
	}
}
