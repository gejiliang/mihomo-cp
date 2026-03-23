BINARY := mihomo-cp
CMD     := ./cmd/mihomo-cp

.PHONY: build-backend run test clean

build-backend:
	go build -o bin/$(BINARY) $(CMD)

run: build-backend
	./bin/$(BINARY)

test:
	go test ./...

clean:
	rm -rf bin/
