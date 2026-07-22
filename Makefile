BINARY := go-ovs-monitor
PKG    := ./...

.PHONY: build test vet fmt lint clean install run

build:
	go build -o $(BINARY) .

test:
	go test $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -l -w .

# build + vet + test in one shot
lint: fmt vet test

install:
	go install .

clean:
	rm -f $(BINARY) $(BINARY).exe

# convenience: bring up a demo OVS bridge with traffic, then list it
run: build
	sudo ./scripts/demo-lab.sh up
	sudo ./$(BINARY) bridges
