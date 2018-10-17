# Makefile for building dnsgrpc
BINARY:=dnsgrpc
SYSTEM:=
CHECKS:=check
BUILDOPTS:=-v
GOPATH?=$(HOME)/go
PRESUBMIT:=cmd
MAKEPWD:=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))
CGO_ENABLED:=0

all: dnsgrpc

.PHONY: dnsgrpc
dnsgrpc: $(CHECKS)
	(cd cmd/dnsgrpc && CGO_ENABLED=$(CGO_ENABLED) $(SYSTEM) go build $(BUILDOPTS) -ldflags="-s -w" -o $(BINARY))

.PHONY: check
check: presubmit

# Presubmit runs all scripts in .presubmit; any non 0 exit code will fail the build.
.PHONY: presubmit
presubmit:
	@for pre in $(MAKEPWD)/.presubmit/* ; do "$$pre" $(PRESUBMIT) || exit 1 ; done

.PHONY: clean
clean:
	go clean
	rm -f cmd/dnsgrpc/dnsgrpc

.PHONY: dep-ensure
dep-ensure:
	dep version || go get -u github.com/golang/dep/cmd/dep
	dep ensure -v
	dep prune -v
	find vendor -name '*_test.go' -delete
