GO=/usr/local/go/bin/go

.PHONY: test
test:
	$(GO) test . -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out
	chown ubuntu:ubuntu cover.out

.PHONY: test-etcd
test-etcd:
	$(GO) test . -ginkgo.focus="Etcd" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover-etcd.out
	chown ubuntu:ubuntu cover-etcd.out

.PHONY: test-image
test-image:
	$(GO) test . -ginkgo.focus="Image" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover-image.out
	chown ubuntu:ubuntu cover-image.out

.PHONY: test-dns
test-dns:
	$(GO) test . -ginkgo.focus="InternalDNS" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover-dns.out
	chown ubuntu:ubuntu cover-dns.out

.PHONY: test-ipam
test-ipam:
	$(GO) test . -ginkgo.focus="IPAM" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover-ipam.out
	chown ubuntu:ubuntu cover-ipam.out

.PHONY: test-job
test-job:
	$(GO) test . -ginkgo.focus="Jobs" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover-job.out
	chown ubuntu:ubuntu cover-job.out

.PHONY: test-server
test-server:
	$(GO) test . -ginkgo.focus="Servers" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover-server.out
	chown ubuntu:ubuntu cover-server.out

.PHONY: test-network
test-network:
	$(GO) test . -ginkgo.focus="Networks" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover-network.out
	chown ubuntu:ubuntu cover-network.out

.PHONY: test-volconfig
test-volconfig:
	$(GO) test . -ginkgo.focus="VolumeGroupConfig" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover-vol.out
	chown ubuntu:ubuntu cover-vol.out

.PHONY: test-volumes
test-volumes:
	$(GO) test . -ginkgo.focus="Volumes" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover-volumes.out
	chown ubuntu:ubuntu cover-volumes.out
