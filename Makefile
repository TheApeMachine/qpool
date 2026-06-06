.PHONY: test bench

# qpool uses go:linkname runtime hooks; Go 1.26+ needs this when linking symm.
# export GOFLAGS so make targets and nested go/cgo subprocesses inherit the flag.
# Outside Make, run: export GOFLAGS=-ldflags=-checklinkname=0
# No inner quotes: a single shell layer passes the flag through unambiguously.
export GOFLAGS := -ldflags=-checklinkname=0

LDFLAGS := $(GOFLAGS)

test:
	go test $(LDFLAGS) -v ./...

bench:
	go test $(LDFLAGS) -bench=. ./...
