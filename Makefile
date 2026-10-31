install:
	go install ./cmd/jp

run:
	go run ./cmd/jp

debug:
	go run -tags debug ./cmd/jp

test:
	go test ./...

bench:
	go run ./bench gen 500000 > /tmp/jp-bench.json
	go run ./bench run /tmp/jp-bench.json

update-jsonpath-cts:
	curl https://raw.githubusercontent.com/jsonpath-standard/jsonpath-compliance-test-suite/main/cts.json -o transform/jsonpath/cts.json
