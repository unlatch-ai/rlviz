.PHONY: build check clean dev e2e-flows format gallery lint site test wasm-check webapp webapp-e2e webapp-install webapp-test web-build web-e2e web-install

build: web-build
	mkdir -p bin
	go build -o bin/rlviz ./cmd/rlviz

web-install:
	npm --prefix web ci

webapp-install:
	npm --prefix webapp ci

wasm-check:
	mkdir -p build/wasm-check
	GOOS=js GOARCH=wasm go build ./internal/model ./internal/analyzers ./internal/alignment ./internal/browsercore
	GOOS=js GOARCH=wasm go build -o build/wasm-check/rlviz-browser-core.wasm ./cmd/rlviz-wasm

webapp: web-install webapp-install
	npm --prefix webapp run build
	GOOS=js GOARCH=wasm go build -o webapp/dist/rlviz.wasm ./cmd/rlviz-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" webapp/dist/wasm_exec.js
	cp webapp/vercel.json webapp/dist/vercel.json

webapp-test: webapp-install
	npm --prefix webapp test

webapp-e2e: webapp
	npm --prefix webapp run test:e2e

e2e-flows: web-build webapp
	cd web && npm exec playwright test e2e/flow-runner.spec.ts
	cd webapp && npm exec playwright test e2e/flow-runner.spec.ts

web-build:
	npm --prefix web run build

web-e2e:
	npm --prefix web run test:e2e

gallery:
	go run ./cmd/gallerygen

site:
	go run ./cmd/sitegen

test:
	go test ./...
	cd third_party/bubbletea && go test ./...
	npm --prefix web test
	$(MAKE) webapp-test
	npm --prefix packages/npm test
	./scripts/install_test.sh
	./scripts/render_homebrew_formula_test.sh

format:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found; running go vet and gofmt fallback"; \
		go vet ./...; \
		unformatted="$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"; \
		test -z "$$unformatted" || { echo "$$unformatted"; exit 1; }; \
	fi

check: lint
	go test ./...
	cd third_party/bubbletea && go test ./...
	npm --prefix web test
	$(MAKE) webapp-test
	npm --prefix packages/npm test
	npm --prefix web run build
	$(MAKE) wasm-check
	$(MAKE) webapp
	./scripts/install_test.sh
	./scripts/render_homebrew_formula_test.sh
	$(MAKE) e2e-flows

dev:
	npm --prefix web run dev

clean:
	rm -rf bin build web/dist webapp/dist webapp/node_modules
