all: build/qbridge

build/qbridge: qbridge/qbridge.go | build
	go build -o $@ $<

build:
	mkdir build

clean:
	rm -rf build
