.PHONY: test
test:
	gomod tidy
	ginkgo -v --race --focus-file mock-server_test.go
	
