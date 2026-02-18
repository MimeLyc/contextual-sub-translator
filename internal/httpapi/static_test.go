package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

func TestServer_ServesSPAFromStaticDir(t *testing.T) {
	tmp := t.TempDir()
	staticDir := filepath.Join(tmp, "web")
	require.NoError(t, os.MkdirAll(staticDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>spa</html>"), 0o644))

	sourcePath := filepath.Join(tmp, "tvshows")
	require.NoError(t, os.MkdirAll(sourcePath, 0o755))

	scanner := library.NewScanner(
		[]library.SourceConfig{{ID: "tvshows", Name: "TV Shows", Path: sourcePath}},
		language.Chinese,
	)
	server := NewServer(scanner, jobs.NewQueue(1), WithUI(staticDir, true))

	for _, url := range []string{"/", "/series/abc"} {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()

		server.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "spa")
	}
}
