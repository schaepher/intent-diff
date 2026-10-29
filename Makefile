BIN ?= bin/intent-review
GO ?= go
PLAYWRIGHT_HTML ?= .tmp/demo/out/review.html

.PHONY: build test vet fmt-check demo ui-check verify clean

build:
	$(GO) build -o $(BIN) ./cmd/intent-review

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt-check:
	@out="$$(gofmt -l cmd internal)"; \
	if [ -n "$$out" ]; then echo "需要 gofmt:"; echo "$$out"; exit 1; fi

demo:
	bash examples/demo/run.sh

# 需要先 npm i playwright（或在可解析 playwright 的目录执行）
ui-check: build demo
	node scripts/ui-check.mjs "$(PLAYWRIGHT_HTML)"

verify:
	bash scripts/verify.sh

clean:
	rm -rf bin .tmp
