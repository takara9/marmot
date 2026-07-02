package controller

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("server controller helpers", func() {
	Describe("clusterHasAnyNode", func() {
		DescribeTable("cluster node availability",
			func(statuses []api.HostStatus, want bool) {
				Expect(clusterHasAnyNode(statuses)).To(Equal(want))
			},
			Entry("has valid node", []api.HostStatus{{NodeName: util.StringPtr("hvc")}}, true),
			Entry("node name with spaces only", []api.HostStatus{{NodeName: util.StringPtr("   ")}, {}}, false),
			Entry("empty statuses", []api.HostStatus{}, false),
		)
	})

	Describe("shouldBypassNodeGateForDeletingServer", func() {
		deletingStatus := &api.Status{StatusCode: db.SERVER_DELETING}
		runningStatus := &api.Status{StatusCode: db.SERVER_RUNNING}
		statusesWithNode := []api.HostStatus{{NodeName: util.StringPtr("hvc")}}
		emptyStatuses := []api.HostStatus{}

		DescribeTable("bypass behavior by status and assignment",
			func(spec api.Server, statuses []api.HostStatus, wantBypass bool, wantReason string) {
				gotBypass, gotReason := shouldBypassNodeGateForDeletingServer(spec, statuses)
				Expect(gotBypass).To(Equal(wantBypass))
				Expect(gotReason).To(Equal(wantReason))
			},
			Entry("not deleting", api.Server{Status: runningStatus}, statusesWithNode, false, ""),
			Entry("cluster empty", api.Server{Status: deletingStatus}, emptyStatuses, true, "cluster_nodes_empty"),
			Entry("assigned node not found", api.Server{
				Status: deletingStatus,
				Metadata: api.Metadata{
					NodeName: util.StringPtr("ws1"),
				},
			}, statusesWithNode, true, "assigned_node_not_found"),
			Entry("assigned node found", api.Server{
				Status: deletingStatus,
				Metadata: api.Metadata{
					NodeName: util.StringPtr("hvc"),
				},
			}, statusesWithNode, false, ""),
		)
	})

	Describe("isRetryableServerProvisionError", func() {
		DescribeTable("retryable classification",
			func(err error, want bool) {
				Expect(isRetryableServerProvisionError(err)).To(Equal(want))
			},
			Entry("network missing is retryable", errors.New("network 'webservers-net' is not found"), true),
			Entry("network ipam not ready is retryable", errors.New("network 'private-net' is not ready for IP allocation"), true),
			Entry("ovs bridge device missing is retryable", errors.New("virError(Code=38, Domain=0, Message='Cannot get interface MTU on 'ovsbr0': No such device')"), true),
			Entry("overlay bridge ensure error is retryable", errors.New("failed to ensure bridge for network test-net-3: exit status 1"), true),
			Entry("bridge device not ready text is retryable", errors.New("bridge device not ready; retrying after ensuring server network dependencies"), true),
			Entry("generic ovs no such device is retryable", errors.New("failed to attach nic on ovsbr0: no such device"), true),
			Entry("ssh key fetch transport error is retryable", errors.New("failed to fetch keys from https://github.com/foo.keys: Get \"https://github.com/foo.keys\": dial tcp: i/o timeout"), true),
			Entry("ssh key fetch http status error is retryable", errors.New("unexpected HTTP status 503 from https://github.com/foo.keys"), true),
			Entry("ssh key empty response is retryable", errors.New("no public keys found at https://github.com/foo.keys"), true),
			Entry("missing qcow2 source file copy is retryable", errors.New("failed to copy QCOW2 volume from /var/lib/marmot/images/x/osimage-x.qcow2 to /var/lib/marmot/volumes/boot-y.qcow2: exit status 1 (output: cp: cannot stat '/var/lib/marmot/images/x/osimage-x.qcow2': No such file or directory)"), true),
			Entry("other provisioning error is not retryable", errors.New("boot volume path is required for qcow2"), false),
			Entry("nil error is not retryable", nil, false),
		)
	})
})
