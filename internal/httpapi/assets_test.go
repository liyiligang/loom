package httpapi_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/liyiligang/loom/internal/httpapi"
)

func TestThemeStartupScriptCanRefreshWithoutWeakeningCSP(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    {Data: []byte(`<script src="/theme.js"></script>`)},
		"theme.js":      {Data: []byte(`document.documentElement.dataset.theme = "dark"`)},
		"assets/app.js": {Data: []byte(`console.log("app")`)},
	}
	handler := httpapi.New(context.Background(), nil, nil, assets, "127.0.0.1:7331")
	for _, file := range []string{"/", "/theme.js", "/assets/app.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:7331"+file, nil))
		if response.Code != 200 {
			t.Fatalf("%s returned %d", file, response.Code)
		}
		cache := response.Header().Get("Cache-Control")
		if file == "/assets/app.js" {
			if !strings.Contains(cache, "immutable") {
				t.Fatal("fingerprinted application assets lost immutable caching")
			}
		} else if cache != "no-cache" {
			t.Fatalf("%s must revalidate after an update: %s", file, cache)
		}
		if file == "/" && !strings.Contains(response.Header().Get("Content-Security-Policy"), "script-src 'self';") {
			t.Fatal("the startup script must work without enabling inline scripts")
		}
		if file == "/theme.js" && !strings.Contains(response.Header().Get("Content-Type"), "javascript") {
			t.Fatal("the startup script has the wrong content type")
		}
	}
}
