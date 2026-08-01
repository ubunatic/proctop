BINARY := proctop

.PHONY: ⚙️ 🤖  # ⚙️ = manual/once, 🤖 = managed

_prim := \033[36m
_rst  := \033[0m

help: 🤖  # show this help
	@grep -E '^[a-zA-Z_-]+:.*[⚙🤖].*#+' $(MAKEFILE_LIST) | \
	awk 'BEGIN {FS = ":.*#+ "}; {printf "    $(_prim)%-15s$(_rst) %s\n", $$1, $$2}'

build: ⚙️  # build the proctop binary
	go build -o $(BINARY) .

test: ⚙️  # run all tests
	go test ./...

vet: ⚙️  # run go vet
	go vet ./...

validate-spec: ⚙️  # run spec integrity tests
	go test ./spec/...

check: ⚙️ vet test  # vet + tests

reuse: ⚙️  # verify license compliance linting
	reuse lint

install: ⚙️  # install proctop into GOBIN
	go install .

clean: ⚙️  # remove build output
	rm -f $(BINARY)
