.PHONY: demo check typecheck generate

# The whole demo: start the server, call it three times, show what breaks.
demo:
	./scripts/demo.sh

# The server-side defence: check real response bytes against the spec.
check:
	cd backend && go test ./... -run TestContractCheck -v

# The client-side view: the types have nothing to complain about.
typecheck:
	cd frontend && pnpm typecheck && echo "tsc --strict: no errors"

# Re-run both generators. Only needed if you edit openapi.yaml.
generate:
	cd backend && oapi-codegen --config api/cfg.yaml ../openapi.yaml
	cd frontend && pnpm generate
