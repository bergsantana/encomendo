package handler_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
)

// specPath is the contract, from this package's directory.
const specPath = "../../../../../docs/api/openapi.yaml"

var loadSpec = sync.OnceValues(func() (routers.Router, error) {
	doc, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, err
	}
	// Match operations on method and path alone, whatever host a test uses.
	doc.Servers = openapi3.Servers{{URL: "/"}}
	return legacyrouter.NewRouter(doc)
})

// checkContract fails t unless rec is an answer the contract documents for
// req's operation: a listed status, the listed headers, and a body that
// fits its schema (backend/CLAUDE.md, Testing).
func checkContract(t *testing.T, req *http.Request, rec *httptest.ResponseRecorder) {
	t.Helper()
	router, err := loadSpec()
	if err != nil {
		t.Fatalf("load the contract: %v", err)
	}
	route, params, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s is not in the contract: %v", req.Method, req.URL.Path, err)
	}
	opts := &openapi3filter.Options{IncludeResponseStatus: true, AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route, Options: opts},
		Status:                 rec.Code,
		Header:                 rec.Header(),
		Body:                   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
		Options:                opts,
	}
	if err := openapi3filter.ValidateResponse(context.Background(), in); err != nil {
		t.Errorf("%s %s answered %d, which the contract does not allow: %v\nbody: %s",
			req.Method, req.URL.Path, rec.Code, err, rec.Body.String())
	}
}
