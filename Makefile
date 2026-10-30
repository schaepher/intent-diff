BIN ?= bin/intent-review
GO ?= go
PREFIX ?=
PLAYWRIGHT_HTML ?= .tmp/demo/out/review.html

.PHONY: build install test vet fmt-check demo ui-check verify clean

build:
	$(GO) build -o $(BIN) ./cmd/intent-review

# 安装：默认 go install 到 GOBIN/GOPATH/bin（无需 sudo）；
# 指定 PREFIX 时构建并复制到 PREFIX/intent-review（如 /usr/local/bin）。
install:
	@if [ -n "$(PREFIX)" ]; then \
		mkdir -p "$(PREFIX)"; \
		$(GO) build -trimpath -o "$(PREFIX)/intent-review" ./cmd/intent-review; \
		echo "install: ok → $(PREFIX)/intent-review"; \
	else \
		$(GO) install -trimpath ./cmd/intent-review; \
		dir="$$($(GO) env GOBIN)"; \
		[ -n "$$dir" ] || dir="$$($(GO) env GOPATH)/bin"; \
		echo "install: ok → $$dir/intent-review"; \
	fi

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
