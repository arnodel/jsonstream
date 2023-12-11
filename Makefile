install:
	go install ./cmd/jp

run:
	go run ./cmd/jp

debug:
	go run -tags debug ./cmd/jp

test:
	go test ./...
