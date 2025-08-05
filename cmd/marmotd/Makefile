.PHONY: test
test:
	go mod tidy
	ginkgo -v --race --focus-file mock-server_test.go
	
