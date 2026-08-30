SHELL := /bin/sh
.DEFAULT_GOAL := help
.NOTPARALLEL:

TOOLKIT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
SKILL_DIR := $(abspath $(TOOLKIT_DIR)/../..)
REPO_DIR := $(abspath $(SKILL_DIR)/../..)
CACHE_DIR ?= $(abspath $(REPO_DIR)/../_workspace/.cache/lodestone)
GOLANGCI_LINT := $(CACHE_DIR)/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.13.1
JSCPD := $(CACHE_DIR)/jscpd/node_modules/.bin/jscpd
JSCPD_VERSION ?= 5.0.16
UV_CACHE_DIR := $(CACHE_DIR)/uv-cache
HF_HOME := $(CACHE_DIR)/huggingface
HF_CLI_VERSION ?= 1.29.0
QWEN_MLX_LM_VERSION ?= 0.31.3
QWEN_MODEL_REPOSITORY ?= Qwen/Qwen3-Embedding-0.6B
QWEN_MODEL_REVISION ?= 97b0c614be4d77ee51c0cef4e5f07c00f9eb65b3
GRANITE_CONVERTER_REPOSITORY ?= https://github.com/pappitti/mlx-raclate.git
GRANITE_CONVERTER_REVISION ?= 2a85e79035da8f6fc06cfd865639ed5e5a16cdb5
GRANITE_MODEL_REPOSITORY ?= ibm-granite/granite-embedding-97m-multilingual-r2
GRANITE_MODEL_REVISION ?= 835ad14087e140460703cf0fae09f97d469d65c2
EMBEDDING_MODEL ?= qwen3

ifeq ($(EMBEDDING_MODEL),qwen3)
MODEL_REPOSITORY := $(QWEN_MODEL_REPOSITORY)
MODEL_REVISION := $(QWEN_MODEL_REVISION)
MODEL_SOURCE := $(CACHE_DIR)/sources/Qwen3-Embedding-0.6B-source
MODEL := $(CACHE_DIR)/models/Qwen3-Embedding-0.6B-8bit-mlx
MODEL_DIMENSION := 1024
MODEL_DEFAULT_MIN_SCORE := 0.65
MODEL_CACHE_NAME := qwen3-embedding.gob
MODEL_CONVERTER := $(TOOLKIT_DIR)/scripts/setup/convert_qwen.py
MODEL_INSTRUCTION := Retrieve functions with the same inputs, outputs, and side effects
else ifeq ($(EMBEDDING_MODEL),granite)
MODEL_REPOSITORY := $(GRANITE_MODEL_REPOSITORY)
MODEL_REVISION := $(GRANITE_MODEL_REVISION)
MODEL_SOURCE := $(CACHE_DIR)/sources/granite-embedding-97m-multilingual-r2-source
MODEL := $(CACHE_DIR)/models/granite-embedding-97m-multilingual-r2-8bit-mlx
MODEL_DIMENSION := 384
MODEL_DEFAULT_MIN_SCORE := 0.80
MODEL_CACHE_NAME := granite-embedding-97m-multilingual-r2.gob
MODEL_CONVERTER := $(TOOLKIT_DIR)/scripts/setup/convert_granite.py
MODEL_INSTRUCTION :=
else
$(error EMBEDDING_MODEL must be qwen3 or granite)
endif

RUNTIME := $(CACHE_DIR)/swift-build/release/lodestone
METALLIB := $(CACHE_DIR)/swift-build/release/mlx.metallib
ANALYZER := $(CACHE_DIR)/bin/code-similarity
EMBEDDING_CACHE ?= $(CACHE_DIR)/embeddings/$(MODEL_CACHE_NAME)
INSTRUCTION ?= $(MODEL_INSTRUCTION)
REPOSITORY ?=
PATTERNS ?= ./...
OUTPUT ?=
JSCPD_REPORT ?=
MIN_SCORE ?= $(MODEL_DEFAULT_MIN_SCORE)
LIMIT ?= 50
CANDIDATE_POOL ?= 300
RRF_K ?= 60
PAUSE_MS ?= 100
MIN_TOKENS ?= 50
MIN_LINES ?= 6
MIN_NODES ?= 3
BATCH ?= 8
MAX_TOKENS ?= 768
CHUNK_TOKENS ?=
SPLIT_AFTER ?=
FORMAT ?= markdown
INCLUDE_TESTS ?=
DUPL_THRESHOLD ?= 150
SOURCE_LINES ?= 80
INCLUDE_GENERATED ?=
DETECTOR_WORK_DIR ?= $(CACHE_DIR)/detectors
DEVELOPER_DIR ?= $(if $(wildcard /Applications/Xcode.app/Contents/Developer/usr/bin/xcodebuild),/Applications/Xcode.app/Contents/Developer,$(shell xcode-select -p))

.PHONY: help model build analyzer analyze require-repository smoke lint lint-go lint-copy-paste lint-converter test check doctor

help:
	@printf '%s\n' \
		'Usage: make <target> [EMBEDDING_MODEL=qwen3|granite]' \
		'' \
		'  qwen3 is the default; pass EMBEDDING_MODEL=granite to select Granite' \
		'' \
		'  model   Download and convert the selected pinned embedding model' \
		'  build   Build the shared MLX Swift runtime and its Metal library' \
		'  analyzer Build the Go hybrid similarity analyzer' \
		'  analyze Combine semantic, jscpd, and dupl candidate ranks (REPOSITORY= required)' \
		'  smoke   Load the selected model and verify one embedding response' \
		'  lint    Run golangci-lint, jscpd, and converter syntax checks' \
		'  test    Test and vet the Go analyzer' \
		'  check   Run lint, tests and vet' \
		'  doctor  Check selected model and runtime artifacts'

$(MODEL_SOURCE)/.complete:
	@mkdir -p "$(MODEL_SOURCE)" "$(UV_CACHE_DIR)" "$(HF_HOME)"
	@UV_CACHE_DIR="$(UV_CACHE_DIR)" HF_HOME="$(HF_HOME)" \
		uvx --from "huggingface-hub==$(HF_CLI_VERSION)" hf download \
		"$(MODEL_REPOSITORY)" --revision "$(MODEL_REVISION)" \
		--local-dir "$(MODEL_SOURCE)" --max-workers 2 --format quiet
	@printf '%s\n' "$(MODEL_REVISION)" > "$(MODEL_SOURCE)/.revision"
	@touch "$@"

ifeq ($(EMBEDDING_MODEL),qwen3)
$(MODEL)/.complete: $(MODEL_SOURCE)/.complete
	@if test ! -f "$(MODEL_SOURCE)/.revision"; then \
		printf '%s\n' "$(MODEL_REVISION)" > "$(MODEL_SOURCE)/.revision"; \
	elif test "$$(cat "$(MODEL_SOURCE)/.revision")" != "$(MODEL_REVISION)"; then \
		echo "Source snapshot revision $$(cat "$(MODEL_SOURCE)/.revision") does not match the pinned $(MODEL_REVISION); remove $(MODEL_SOURCE) to re-download." >&2; \
		exit 1; \
	fi
	@test ! -e "$(MODEL)" || { \
		echo "Model directory exists but is incomplete: $(MODEL)" >&2; \
		exit 1; \
	}
	@mkdir -p "$(dir $(MODEL))" "$(UV_CACHE_DIR)" "$(HF_HOME)"
	@UV_CACHE_DIR="$(UV_CACHE_DIR)" HF_HOME="$(HF_HOME)" \
		uvx --from "mlx-lm==$(QWEN_MLX_LM_VERSION)" \
		python "$(MODEL_CONVERTER)" \
		--source "$(MODEL_SOURCE)" --output "$(MODEL)" \
		--repository "$(MODEL_REPOSITORY)" --revision "$(MODEL_REVISION)" \
		--group-size 64 --bits 8
	@touch "$@"
else
$(MODEL)/.complete: $(MODEL_SOURCE)/.complete
	@if test ! -f "$(MODEL_SOURCE)/.revision"; then \
		printf '%s\n' "$(MODEL_REVISION)" > "$(MODEL_SOURCE)/.revision"; \
	elif test "$$(cat "$(MODEL_SOURCE)/.revision")" != "$(MODEL_REVISION)"; then \
		echo "Source snapshot revision $$(cat "$(MODEL_SOURCE)/.revision") does not match the pinned $(MODEL_REVISION); remove $(MODEL_SOURCE) to re-download." >&2; \
		exit 1; \
	fi
	@test ! -e "$(MODEL)" || { \
		echo "Model directory exists but is incomplete: $(MODEL)" >&2; \
		exit 1; \
	}
	@mkdir -p "$(dir $(MODEL))" "$(UV_CACHE_DIR)" "$(HF_HOME)"
	@UV_CACHE_DIR="$(UV_CACHE_DIR)" HF_HOME="$(HF_HOME)" \
		uvx --from "git+$(GRANITE_CONVERTER_REPOSITORY)@$(GRANITE_CONVERTER_REVISION)" \
		python "$(MODEL_CONVERTER)" \
		--source "$(MODEL_SOURCE)" --output "$(MODEL)" \
		--repository "$(MODEL_REPOSITORY)" --revision "$(MODEL_REVISION)" \
		--converter-revision "$(GRANITE_CONVERTER_REVISION)" \
		--group-size 64 --bits 8
	@touch "$@"
endif

model:
	@if test ! -f "$(MODEL)/.complete"; then \
		$(MAKE) -C "$(TOOLKIT_DIR)" EMBEDDING_MODEL="$(EMBEDDING_MODEL)" "$(MODEL)/.complete"; \
	fi
	@printf '%s\n' "Embedding model ready ($(EMBEDDING_MODEL)): $(MODEL)"

$(RUNTIME): $(TOOLKIT_DIR)/runtime/Package.swift $(TOOLKIT_DIR)/runtime/Package.resolved $(shell find "$(TOOLKIT_DIR)/runtime/Sources" -type f -name '*.swift')
	@DEVELOPER_DIR="$(DEVELOPER_DIR)" swift build --package-path "$(TOOLKIT_DIR)/runtime" --scratch-path "$(CACHE_DIR)/swift-build" --configuration release --jobs 1

$(METALLIB): $(RUNTIME) $(TOOLKIT_DIR)/scripts/setup/build_metallib.sh
	@DEVELOPER_DIR="$(DEVELOPER_DIR)" BUILD_DIR="$(CACHE_DIR)/swift-build" UV_CACHE_DIR="$(UV_CACHE_DIR)" "$(TOOLKIT_DIR)/scripts/setup/build_metallib.sh" release

build: $(RUNTIME) $(METALLIB)
	@printf '%s\n' "Embedding runtime ready: $(RUNTIME)"

$(ANALYZER): $(shell find "$(TOOLKIT_DIR)/harness/cmd" "$(TOOLKIT_DIR)/harness/internal" -type f -name '*.go') $(TOOLKIT_DIR)/harness/go.mod
	@mkdir -p "$(dir $(ANALYZER))"
	@GOMAXPROCS=2 go build -C "$(TOOLKIT_DIR)/harness" -o "$(ANALYZER)" ./cmd/code-similarity

analyzer: $(ANALYZER)
	@printf '%s\n' "Hybrid similarity analyzer ready: $(ANALYZER)"

require-repository:
	@test -n "$(REPOSITORY)" || { echo 'REPOSITORY is required.' >&2; exit 1; }

analyze: require-repository model $(RUNTIME) $(METALLIB) analyzer $(JSCPD)
	@"$(ANALYZER)" \
		--root "$(REPOSITORY)" \
		--patterns "$(PATTERNS)" \
		--runtime "$(RUNTIME)" \
		--model "$(MODEL)" \
		--model-profile "$(EMBEDDING_MODEL)" \
		--cache "$(EMBEDDING_CACHE)" \
		--instruction "$(INSTRUCTION)" \
		--min-score "$(MIN_SCORE)" \
		--limit "$(LIMIT)" \
		--candidate-pool "$(CANDIDATE_POOL)" \
		--rrf-k "$(RRF_K)" \
		--pause-ms "$(PAUSE_MS)" \
		--min-tokens "$(MIN_TOKENS)" \
		--min-lines "$(MIN_LINES)" \
		--min-nodes "$(MIN_NODES)" \
		--batch "$(BATCH)" \
		--max-tokens "$(MAX_TOKENS)" \
		--format "$(FORMAT)" \
		--source-lines "$(SOURCE_LINES)" \
		--work-dir "$(DETECTOR_WORK_DIR)" \
		--jscpd "$(JSCPD)" \
		--dupl-threshold "$(DUPL_THRESHOLD)" \
		$(if $(CHUNK_TOKENS),--chunk-tokens "$(CHUNK_TOKENS)") \
		$(if $(SPLIT_AFTER),--split-after "$(SPLIT_AFTER)") \
		$(if $(INCLUDE_TESTS),--include-tests) \
		$(if $(INCLUDE_GENERATED),--include-generated) \
		$(if $(OUTPUT),--output "$(OUTPUT)") \
		$(if $(JSCPD_REPORT),--jscpd-report "$(JSCPD_REPORT)")

smoke: model build
	@printf '%s\n' '{"id":1,"texts":["func add(a, b int) int { return a + b }"]}' | \
		"$(RUNTIME)" --model "$(MODEL)" --max-batch 1 --max-tokens 64 | \
		grep -q '"dimension":$(MODEL_DIMENSION)'
	@printf '%s\n' "Embedding smoke test passed ($(EMBEDDING_MODEL), $(MODEL_DIMENSION) dimensions)."

$(GOLANGCI_LINT):
	@mkdir -p "$(dir $@)"
	@GOBIN="$(dir $@)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(JSCPD):
	@mkdir -p "$(CACHE_DIR)/jscpd"
	@npm install --prefix "$(CACHE_DIR)/jscpd" --no-save --no-package-lock --ignore-scripts \
		"jscpd@$(JSCPD_VERSION)"

lint-go: $(GOLANGCI_LINT)
	@cd "$(TOOLKIT_DIR)/harness" && GOMAXPROCS=2 "$(GOLANGCI_LINT)" run --config .golangci.yml --tests=true --concurrency=2 ./...

lint-copy-paste: $(JSCPD)
	@cd "$(SKILL_DIR)" && "$(JSCPD)" --config .jscpd.json --workers 2 --threshold 0

lint-converter:
	@PYTHONPYCACHEPREFIX="$(CACHE_DIR)/pycache" python3 -m py_compile $(TOOLKIT_DIR)/scripts/setup/convert_common.py $(TOOLKIT_DIR)/scripts/setup/convert_qwen.py $(TOOLKIT_DIR)/scripts/setup/convert_granite.py

lint: lint-go lint-copy-paste lint-converter

test:
	@GOMAXPROCS=2 go test -C "$(TOOLKIT_DIR)/harness" ./...
	@GOMAXPROCS=2 go vet -C "$(TOOLKIT_DIR)/harness" ./...

check: lint test

doctor:
	@test -f "$(MODEL)/.complete" || { echo 'Embedding model completion marker is missing.' >&2; exit 1; }
	@test -s "$(MODEL)/model.safetensors" || { echo 'Embedding model is not ready.' >&2; exit 1; }
	@grep -q '"repository": "$(MODEL_REPOSITORY)"' "$(MODEL)/conversion.json" || { echo 'Embedding model repository pin does not match.' >&2; exit 1; }
	@grep -q '"revision": "$(MODEL_REVISION)"' "$(MODEL)/conversion.json" || { echo 'Embedding model revision pin does not match.' >&2; exit 1; }
	@test -x "$(RUNTIME)" || { echo 'Embedding runtime is not built.' >&2; exit 1; }
	@test -s "$(METALLIB)" || { echo 'MLX Metal library is not built.' >&2; exit 1; }
	@printf '%s\n' "Embedding model and runtime are ready ($(EMBEDDING_MODEL))."
