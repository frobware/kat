.PHONY: build
build:
	go fmt ./...
	go vet ./...
	go test ./...
	go build ./cmd/kat

.PHONY: clean
clean:
	$(RM) kat
