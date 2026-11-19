.PHONY: all build test clean simulation demo web-build

BINARY_NAME=embshift

all: build test

web-build:
	cd web && npm install && npm run build

build: web-build
	go build -ldflags="-s -w" -o $(BINARY_NAME).exe ./cmd/embshift

test:
	go test -v -race ./...

simulation:
	go run ./examples/simulation.go

demo: build
	./$(BINARY_NAME).exe demo

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME).exe
	rm -f *.db *.db-wal *.db-shm audit.json
	rm -rf web/dist
