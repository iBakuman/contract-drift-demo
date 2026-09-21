package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	"github.com/iBakuman/contract-drift-demo/backend/api"
	"github.com/iBakuman/contract-drift-demo/backend/contractcheck"
)

// TestContractCheck shows what checking real response bytes against the spec
// does and does not catch.
func TestContractCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(listWidgets))
	defer srv.Close()

	doc, err := api.GetSwagger()
	require.NoError(t, err)
	// The document declares a fixed server URL; the test server is on a random
	// port, so point the document at it before building the router.
	doc.Servers = openapi3.Servers{{URL: srv.URL}}

	checker, err := contractcheck.New(doc, nil)
	require.NoError(t, err)
	client := &http.Client{Transport: checker}

	t.Run("ok: the well-behaved handler", func(t *testing.T) {
		checker.Reset()
		body := get(t, client, srv.URL, "ok")
		t.Logf("response bytes: %s", body)
		require.Empty(t, checker.Violations, "well-behaved response should satisfy the contract")
	})

	t.Run("failure mode A: server emits a null the contract forbids", func(t *testing.T) {
		checker.Reset()
		body := get(t, client, srv.URL, "nil-slice")
		t.Logf("response bytes: %s", body)

		require.JSONEq(t, `{"generatedAt":"redacted","items":null}`, redactTimestamp(t, body),
			"the bug reproduces: items is null on the wire")

		require.Len(t, checker.Violations, 1, "the checker should catch this")
		t.Logf("violation: %v", checker.Violations[0].Err)
		require.Contains(t, checker.Violations[0].Err.Error(), "not nullable")
	})

	t.Run("failure mode B: server legally omits an optional field", func(t *testing.T) {
		checker.Reset()
		body := get(t, client, srv.URL, "omit-optional")
		t.Logf("response bytes: %s", body)

		require.NotContains(t, string(body), `"label"`, "label is absent from the wire")

		// This is the boundary. The response is contract-legal, so nothing here
		// is violated — and yet a client that reads label without checking will
		// still break. Response validation is the wrong defence for this one.
		require.Empty(t, checker.Violations,
			"omitting an optional field is legal; the checker is blind to mode B by design")
	})
}

func get(t *testing.T, client *http.Client, base, scenario string) []byte {
	t.Helper()

	resp, err := client.Get(fmt.Sprintf("%s/widgets?scenario=%s", base, scenario))
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	return body
}

func redactTimestamp(t *testing.T, body []byte) string {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	m["generatedAt"] = "redacted"

	out, err := json.Marshal(m)
	require.NoError(t, err)

	return string(out)
}
