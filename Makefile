.PHONY: build check clean dev format test web-build web-e2e web-install

build: web-build
	mkdir -p bin
	go build -o bin/rlviz ./cmd/rlviz

web-install:
	npm --prefix web ci

web-build:
	npm --prefix web run build

web-e2e:
	npm --prefix web run test:e2e

test:
	go test ./...
	npm --prefix web test
	npm --prefix packages/npm test
	./scripts/install_test.sh
	./scripts/render_homebrew_formula_test.sh

format:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

check:
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"
	go vet ./...
	go test ./...
	npm --prefix web test
	npm --prefix packages/npm test
	npm --prefix web run build
	./scripts/install_test.sh
	./scripts/render_homebrew_formula_test.sh

dev:
	npm --prefix web run dev

clean:
	rm -rf bin web/dist
