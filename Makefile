.PHONY: demo check generate typecheck clean

# The whole demo: start the server, run the client against all three
# scenarios, stop the server.
demo:
	@cd backend && go build -o /tmp/contract-drift-demo-server .
	@/tmp/contract-drift-demo-server & echo $$! > /tmp/contract-drift-demo.pid; sleep 1
	@cd frontend && pnpm --silent consume || true
	@kill `cat /tmp/contract-drift-demo.pid` 2>/dev/null; rm -f /tmp/contract-drift-demo.pid

# The server-side defence: check real response bytes against the spec.
check:
	cd backend && go test ./... -run TestContractCheck -v

# The client-side defence: the types say nothing is wrong.
typecheck:
	cd frontend && pnpm --silent typecheck && echo "tsc --strict: no errors"

# Re-run both generators. Only needed if you edit openapi.yaml.
generate:
	cd backend && oapi-codegen --config api/cfg.yaml ../openapi.yaml
	cd frontend && pnpm --silent generate

clean:
	rm -f /tmp/contract-drift-demo-server /tmp/contract-drift-demo.pid
