.PHONY: build run clean test bench tidy

BINARY := hifi-tui
CMD    := ./cmd/hifi-tui

build:
	go build -o $(BINARY) $(CMD)

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)

test:
	go test ./internal/...

bench:
	go test -bench=. -benchmem ./internal/download/...

tidy:
	go mod tidy
