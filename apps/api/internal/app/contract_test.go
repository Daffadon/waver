package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"github.com/daffadon/waver/internal/flag"
	"github.com/daffadon/waver/internal/iam"
	"github.com/daffadon/waver/internal/ofrep"
	"github.com/daffadon/waver/internal/shared"
)

// openapi.yaml is the contract. This test fails when a documented
// route is missing from the router, or a probed route is undocumented.
func TestContract(t *testing.T) {
	spec, err := loadSpec(t)
	if err != nil {
		t.Fatal(err)
	}

	dbf := filepath.Join(t.TempDir(), "contract.db")
	db, err := shared.OpenDB(dbf, "../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := shared.Config{Port: "0", DBPath: dbf, JWTSecret: "contract-secret"}
	ks := &iam.Store{DB: db}
	pw, _ := iam.RandomPassword()
	hh, _ := iam.HashPassword(pw)
	if err := ks.PutAdmin(hh, true); err != nil {
		t.Fatal(err)
	}
	fs := &flag.Store{DB: db}
	h := Router(cfg, &iam.Handler{Store: ks, Cfg: cfg}, &flag.Handler{Store: fs}, &ofrep.Handler{Flags: fs, Keys: ks})

	call := func(method, path, body, tok string) int {
		req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// change-password (no JWT, first-login flow), then login for a token.
	if code := call("POST", "/api/v1/auth/change-password", `{"current":"`+pw+`","next":"contract-pass-123"}`, ""); code != 200 {
		t.Fatalf("change-password: got %d", code)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader([]byte(`{"password":"contract-pass-123"}`))))
	var login struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &login)
	if login.Token == "" {
		t.Fatal("login returned no token")
	}

	// /docs serves the exact contract file clients code against.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/docs/openapi.yaml", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("docs spec: got %d", rec.Code)
	}
	raw, err := os.ReadFile(specPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Body.String() != string(raw) {
		t.Fatal("docs spec differs from packages/contracts/openapi.yaml")
	}

	// one API key for the OFREP probes.
	var created struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/apikeys", bytes.NewReader([]byte(`{"name":"contract"}`)))
	req.Header.Set("Authorization", "Bearer "+login.Token)
	h.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	probes := map[string]struct {
		body string
		tok  string
		want []int
	}{
		"POST /api/v1/auth/login":                 {`{"password":"x"}`, "", []int{http.StatusUnauthorized, http.StatusForbidden}},
		"POST /api/v1/auth/change-password":      {`{"current":"x","next":"contract-pass-123"}`, "", []int{http.StatusUnauthorized}},
		"GET /api/v1/flags":                      {"", login.Token, []int{http.StatusOK}},
		"POST /api/v1/flags":                     {`{"key":"probe","name":"Probe"}`, login.Token, []int{http.StatusCreated}},
		"POST /ofrep/v1/evaluate/flags/{key}": {`{"context":{"targetingKey":"u1"}}`, created.Key, []int{http.StatusOK}},
		"POST /ofrep/v1/evaluate/flags":          {`{"context":{"targetingKey":"u1"}}`, created.Key, []int{http.StatusOK}},
	}

	for docPath, methods := range spec.Paths {
		for method := range methods {
			path := strings.ReplaceAll(docPath, "{key}", "probe")
			p, ok := probes[strings.ToUpper(method)+" "+docPath]
			if !ok {
				t.Errorf("documented route has no probe: %s %s", method, docPath)
				continue
			}
			code := call(strings.ToUpper(method), path, p.body, p.tok)
			if code == http.StatusNotFound || code == http.StatusMethodNotAllowed {
				t.Errorf("documented route not implemented: %s %s (got %d)", method, docPath, code)
				continue
			}
			allowed := false
			for _, w := range p.want {
				if code == w {
					allowed = true
				}
			}
			if !allowed {
				t.Errorf("probe %s %s: got %d, want one of %v", method, docPath, code, p.want)
			}
		}
	}
}

type specFile struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

var specCandidates = []string{
	"../../../../packages/contracts/openapi.yaml",
	"../../../packages/contracts/openapi.yaml",
}

func specPath(t *testing.T) string {
	t.Helper()
	for _, p := range specCandidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatal("openapi.yaml not found")
	return ""
}

func loadSpec(t *testing.T) (specFile, error) {
	t.Helper()
	p := specPath(t)
	b, err := os.ReadFile(p)
	if err != nil {
		return specFile{}, err
	}
	var s specFile
	if err := yaml.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if len(s.Paths) == 0 {
		t.Fatal("contract has no paths")
	}
	return s, nil
}
