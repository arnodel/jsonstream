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
	go test -run=^$$ -fuzz=FuzzDecoder -fuzztime=$(FUZZTIME) ./encoding/json
	go test -run=^$$ -fuzz=FuzzJPVDecoder -fuzztime=$(FUZZTIME) ./encoding/jpv
	go test -run=^$$ -fuzz=FuzzJPVRoundTrip -fuzztime=$(FUZZTIME) ./encoding/jpv
	go test -run=^$$ -fuzz=FuzzParseQueryString -fuzztime=$(FUZZTIME) ./internal/jsonpath
	go test -run=^$$ -fuzz=FuzzQuery -fuzztime=$(FUZZTIME) ./transform/jsonpath

bench:
	go run ./bench gen 500000 > /tmp/pj-bench.json
	go run ./bench run /tmp/pj-bench.json -svg docs/benchmark.svg

update-jsonpath-cts:
	curl https://raw.githubusercontent.com/jsonpath-standard/jsonpath-compliance-test-suite/main/cts.json -o transform/jsonpath/cts.json
