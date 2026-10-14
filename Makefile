# Partscaster — основные задачи. make help покажет список.
WAILS ?= $(HOME)/go/bin/wails
# версия сборки — из git (тег + коммиты после него); без git — dev
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help test test-go test-front test-worker lint lint-go lint-front lint-worker fmt build install-desktop uninstall-desktop worker worker-tar yue2cpp release dist-linux dist-windows dist-mac mcp mcp-data clean

help: ## показать список задач
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[1m%-14s\033[0m %s\n", $$1, $$2}'

## test: все тесты (Go + фронтенд + воркер)
test: test-go test-front test-worker

test-go: ## тесты Go
	go test ./...

test-front: ## тесты фронтенда (vitest)
	cd frontend && npm test

test-worker: ## тесты воркера (unittest)
	cd worker && python3 -m unittest discover -p 'test_*.py'

## lint: все линтеры (go vet + golangci-lint, eslint, ruff)
lint: lint-go lint-front lint-worker

lint-go: ## линт Go
	gofmt -l . | grep -q . && (gofmt -l . && echo 'запусти: make fmt' && exit 1) || true
	go vet ./...
	command -v golangci-lint >/dev/null && golangci-lint run || echo 'golangci-lint не установлен — пропущено (CI проверит)'

lint-front: ## линт фронтенда (eslint)
	cd frontend && npx eslint src

lint-worker: ## линт воркера (ruff)
	cd worker && if command -v ruff >/dev/null; then ruff check .; else echo 'ruff не установлен — pip install ruff' >&2; exit 1; fi

fmt: ## форматирование Go
	gofmt -w .

build: ## сборка desktop-приложения (wails build)
	$(WAILS) build -ldflags "-X main.version=$(VERSION)"
	@echo "бинарник: $(CURDIR)/build/bin/partscaster"

# ярлык Linux: имя файла = app_id окна (partscaster), по нему GNOME/Wayland
# сопоставляет окно с ярлыком и берёт иконку отсюда, а не из окна
DESKTOP_FILE ?= $(HOME)/.local/share/applications/partscaster.desktop

install-desktop: ## ярлык в меню приложений Linux (на build/bin/partscaster, иконка build/appicon.png)
	@test -x build/bin/partscaster || { echo 'нет build/bin/partscaster — сначала make build'; exit 1; }
	@mkdir -p $(dir $(DESKTOP_FILE))
	@printf '%s\n' '[Desktop Entry]' 'Type=Application' 'Name=Partscaster' \
		'Comment=Генерация и доводка треков YuE' \
		'Exec="$(CURDIR)/build/bin/partscaster"' 'Icon=$(CURDIR)/build/appicon.png' \
		'Terminal=false' 'Categories=AudioVideo;Audio;' 'StartupWMClass=partscaster' > $(DESKTOP_FILE)
	@command -v update-desktop-database >/dev/null && update-desktop-database -q $(dir $(DESKTOP_FILE)) || true
	@echo "ярлык: $(DESKTOP_FILE)"

uninstall-desktop: ## убрать ярлык из меню приложений
	rm -f $(DESKTOP_FILE)

worker: ## деплой воркера на GPU-машину (PARTSCASTER_DEPLOY_HOST=user@gpu-host): архив → install.sh
	./deploy.sh worker

worker-tar: ## архив воркера dist/worker-<версия>.tar.gz (код + install.sh)
	./tools/worker-tar.sh dist $(VERSION)

yue2cpp: ## готовая сборка yue2.cpp (второй движок, эксперимент) в dist/ для архива воркера; нужен docker
	./tools/yue2cpp-build.sh

release: ## релизный пакет под эту ОС в dist/ (приложение + partscaster-mcp + архив воркера)
	WAILS=$(WAILS) ./tools/release.sh $(VERSION)

dist-linux: ## пакет Linux в dist/ (только на Linux)
	TARGET_OS=linux WAILS=$(WAILS) ./tools/release.sh $(VERSION)

dist-windows: ## пакет Windows в dist/ (.zip; собирается и с Linux)
	TARGET_OS=windows WAILS=$(WAILS) ./tools/release.sh $(VERSION)

dist-mac: ## пакет macOS в dist/ (только на Mac)
	TARGET_OS=darwin WAILS=$(WAILS) ./tools/release.sh $(VERSION)

mcp: ## собрать MCP-сервер (build/bin/partscaster-mcp)
	go build -ldflags "-X main.buildHash=$$(go run ./cmd/srchash) -X main.srcDir=$(CURDIR) -X main.version=$(VERSION)" -o build/bin/partscaster-mcp ./cmd/partscaster-mcp
	@echo "mcp-сервер: $(CURDIR)/build/bin/partscaster-mcp (конфигурация клиентов: docs/mcp.md)"

mcp-data: ## перегенерировать данные библиотеки MCP из фронтенда (go:embed)
	./tools/mcp-gendata.sh

clean: ## почистить артефакты сборки фронта
	cd frontend && rm -rf dist
