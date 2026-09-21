// Package contractcheck validates the bytes a server actually sent against the
// OpenAPI document those bytes were supposed to satisfy.
//
// It is an http.RoundTripper, so it goes underneath an existing http.Client and
// every call made through that client is checked without the calling code
// knowing about it.
package contractcheck

import (
	"bytes"
	"io"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// Violation is one response that did not match the contract.
type Violation struct {
	Method string
	Path   string
	Status int
	Err    error
}

// Transport wraps another RoundTripper and records contract violations.
//
// It reports rather than interrupts: a violation is a finding about the
// response, not a reason to fail the request, so the caller still gets its
// response and can still make its own assertions.
type Transport struct {
	Base       http.RoundTripper
	router     routers.Router
	Violations []Violation
}

// New builds a Transport from an OpenAPI document. The document comes from the
// same generated package the server types came from, so there is no second copy
// of the contract to keep in sync.
func New(doc *openapi3.T, base http.RoundTripper) (*Transport, error) {
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		return nil, err
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return &Transport{Base: base, router: router}, nil
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.Base.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return resp, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	if verr := t.validate(req, resp, body); verr != nil {
		t.Violations = append(t.Violations, Violation{
			Method: req.Method,
			Path:   req.URL.Path,
			Status: resp.StatusCode,
			Err:    verr,
		})
	}

	return resp, nil
}

func (t *Transport) validate(req *http.Request, resp *http.Response, body []byte) error {
	route, pathParams, err := t.router.FindRoute(req)
	if err != nil {
		// A response the spec has no route for is not a contract violation —
		// it is a call to something this contract does not describe.
		return nil
	}

	return openapi3filter.ValidateResponse(req.Context(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
		},
		Status: resp.StatusCode,
		Header: resp.Header,
		Body:   io.NopCloser(bytes.NewReader(body)),
	})
}

// Reset clears recorded violations so one Transport can serve several checks.
func (t *Transport) Reset() { t.Violations = nil }
