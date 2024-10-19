TEST_BINARIES := test/exit99 test/hello test/wait

.PHONY: default
default: recur

.PHONY: clean
clean:
	-rm recur $(TEST_BINARIES)

recur: main.go
	CGO_ENABLED=0 go build

.PHONY: test
test: recur $(TEST_BINARIES)
	go test

test/%: test/%.go
	go build -o $@ $<
