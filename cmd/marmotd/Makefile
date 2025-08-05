.PHONY: test
test:
	go mod tidy
	go install github.com/onsi/ginkgo/v2/ginkgo@latest
	ginkgo -v --race --focus-file mock-server_test.go
	
