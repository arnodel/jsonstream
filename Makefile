install:
	go install ./cmd/pj

run:
	go run ./cmd/pj

debug:
	go run -tags debug ./cmd/pj

test:
	go test ./...
