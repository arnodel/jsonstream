install:
	go install ./cmd/jp

run:
	go run ./cmd/jp

debug:
	go run -tags debug ./cmd/jp

test:
	go test ./...

update-jsonpath-cts:
	curl https://raw.githubusercontent.com/jsonpath-standard/jsonpath-compliance-test-suite/main/cts.json -o transform/jsonpath/cts.json
