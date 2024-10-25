TEST_BINARIES := test/exit99 test/hello test/sleep

.PHONY: all
all: README.md recur

.PHONY: clean
clean:
	-rm README.md recur $(TEST_BINARIES)

README.md: README.template.md recur
	go run script/render_template.go < README.template.md > $@

recur: main.go
	CGO_ENABLED=0 go build

.PHONY: release
release:
	go run script/release.go

.PHONY: test
test: recur $(TEST_BINARIES)
	go test

test/exit99: test/exit99.go
	go build -o $@ test/exit99.go

test/hello: test/hello.go
	go build -o $@ test/hello.go

test/sleep: test/sleep.go
	go build -o $@ test/sleep.go
