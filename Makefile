.PHONY:
install:
	@go install ./cmd/lazycontainer

.PHONY:
run:
	@go run ./cmd/lazycontainer