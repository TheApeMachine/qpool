.PHONY: test bench

# The pool package uses go:linkname to access runtime scheduling
# primitives (dropg, readgstatus) for zero-overhead goroutine parking.
# Go 1.26 restricts these by default; -checklinkname=0 preserves access.
LDFLAGS := -ldflags='-checklinkname=0'

test:
	go test $(LDFLAGS) -v ./...

bench:
	go test $(LDFLAGS) -bench=. ./...