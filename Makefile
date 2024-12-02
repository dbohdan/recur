SUBDIR := v2
TEST_BINARIES := test/env test/exit99 test/hello test/sleep

.PHONY: all
all: README.md $(SUBDIR)/recur

.PHONY: clean
clean:
	-rm README.md $(SUBDIR)/recur $(TEST_BINARIES)

README.md: README.template.md $(SUBDIR)/recur
	cd $(SUBDIR) && go run ../script/render_template.go < ../README.template.md > ../$@

$(SUBDIR)/recur: $(SUBDIR)/main.go
	CGO_ENABLED=0 go build -o $@ $(SUBDIR)/main.go

.PHONY: release
release:
	mkdir -p dist/
	cd $(SUBDIR) && go run ../script/release.go && mv dist/* ../dist/ && rmdir dist/

.PHONY: test
test: $(SUBDIR)/recur $(TEST_BINARIES)
	cd $(SUBDIR) && go test

test/env: test/env.go
	go build -o $@ test/env.go

test/exit99: test/exit99.go
	go build -o $@ test/exit99.go

test/hello: test/hello.go
	go build -o $@ test/hello.go

test/sleep: test/sleep.go
	go build -o $@ test/sleep.go
