.PHONY: install run debug test fuzz bench bench-scale pgo

install:
	go install ./cmd/pj

run:
	go run ./cmd/pj

debug:
	go run -tags debug ./cmd/pj

test:
	go test ./...

# Run each fuzz target for a short while. Pass FUZZTIME=5m for a longer run.
FUZZTIME ?= 30s
fuzz:
	go test -run=^$$ -fuzz=^FuzzDecoder$$ -fuzztime=$(FUZZTIME) ./encoding/json
	go test -run=^$$ -fuzz=^FuzzJPVDecoder$$ -fuzztime=$(FUZZTIME) ./encoding/jpv
	go test -run=^$$ -fuzz=^FuzzJPVRoundTrip$$ -fuzztime=$(FUZZTIME) ./encoding/jpv
	go test -run=^$$ -fuzz=^FuzzParseQueryString$$ -fuzztime=$(FUZZTIME) ./internal/jsonpath
	go test -run=^$$ -fuzz=^FuzzQuery$$ -fuzztime=$(FUZZTIME) ./transform/jsonpath

bench:
	go run ./bench gen 500000 > /tmp/pj-bench.json
	go run ./bench run /tmp/pj-bench.json -svg docs/benchmark.svg
	go run ./bench gen 5000 > /tmp/pj-bench-small.json
	go run ./bench jpv /tmp/pj-bench-small.json -svg docs/benchmark-jpv.svg

# Record the profile for profile-guided builds, cmd/pj/default.pgo, which go
# build uses automatically.  It is recorded on Linux, in a container elsewhere:
# Go's CPU profiles on macOS put most samples in system calls, which halves the
# speed-up.
pgo:
ifeq ($(shell uname),Linux)
	go run ./bench pgo cmd/pj/default.pgo
else
	docker run --rm -v "$$PWD":/src -w /src -e GOFLAGS=-buildvcs=false golang:1.27 go run ./bench pgo cmd/pj/default.pgo
endif

# How time and memory grow with the size of the input; takes about 10 minutes.
bench-scale:
	go run ./bench scale run -svg docs/benchmark-scale.svg
	go run ./bench scale jpv -svg docs/benchmark-scale-jpv.svg

# Fetch cts.json from a commit of the compliance test suite (the tip of main
# by default, or e.g. CTS_REF=<sha>) and record that commit in cts.version.
CTS_REPO = https://github.com/jsonpath-standard/jsonpath-compliance-test-suite
CTS_REF ?= main

update-jsonpath-cts:
	sha=$$(git ls-remote $(CTS_REPO) $(CTS_REF) | cut -f1); \
	if [ -z "$$sha" ]; then sha=$(CTS_REF); fi; \
	curl -fsSL https://raw.githubusercontent.com/jsonpath-standard/jsonpath-compliance-test-suite/$$sha/cts.json -o transform/jsonpath/cts.json && \
	echo $$sha > transform/jsonpath/cts.version
