package controller

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
	etcd "go.etcd.io/etcd/client/v3"
)

func waitServerSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for signal")
	}
}

func TestServerControllerWatchScheduling(t *testing.T) {
	stop := make(chan struct{})
	notifications := make(chan struct{}, 1)
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	done := make(chan struct{})
	var calls atomic.Int32
	go func() {
		defer close(done)
		runServerController(stop, notifications, 500*time.Millisecond, 150*time.Millisecond, func() {
			started <- struct{}{}
			if calls.Add(1) == 1 {
				<-release
			}
		})
	}()
	defer func() {
		close(stop)
		waitServerSignal(t, done, time.Second)
	}()

	notifications <- struct{}{}
	waitServerSignal(t, started, 400*time.Millisecond)
	notifications <- struct{}{}
	select {
	case notifications <- struct{}{}:
	default:
	}
	close(release)
	select {
	case <-started:
		t.Fatal("watch notification was not rate limited")
	case <-time.After(75 * time.Millisecond):
	}
	waitServerSignal(t, started, 400*time.Millisecond)
	select {
	case <-started:
		t.Fatal("duplicate notifications triggered another reconcile")
	case <-time.After(200 * time.Millisecond):
	}
	waitServerSignal(t, started, 400*time.Millisecond)
}

func TestServerWatchRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	notifications := make(chan struct{}, 1)
	attempts := make(chan int, 3)
	var count int
	watch := func(ctx context.Context, key string, opts ...etcd.OpOption) etcd.WatchChan {
		count++
		attempts <- count
		ch := make(chan etcd.WatchResponse, 1)
		if count == 1 {
			ch <- etcd.WatchResponse{Canceled: true, CompactRevision: 10}
			close(ch)
		} else {
			ch <- etcd.WatchResponse{Created: true}
			go func() {
				<-ctx.Done()
				close(ch)
			}()
		}
		return ch
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchServerChanges(ctx, watch, notifications, 10*time.Millisecond)
	}()
	defer func() {
		cancel()
		waitServerSignal(t, done, time.Second)
	}()

	select {
	case <-attempts:
	case <-time.After(time.Second):
		t.Fatal("watch did not start")
	}
	select {
	case n := <-attempts:
		if n != 2 {
			t.Fatalf("unexpected watch attempt %d", n)
		}
	case <-time.After(time.Second):
		t.Fatal("watch did not restart after compaction")
	}
	waitServerSignal(t, notifications, time.Second)
}

func TestServerWatchEtcd(t *testing.T) {
	binary, err := exec.LookPath("etcd")
	if err != nil {
		t.Skip("etcd binary not available")
	}
	freeAddress := func() string {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := listener.Addr().String()
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		return "http://" + address
	}
	clientURL, peerURL := freeAddress(), freeAddress()
	dataDir := t.TempDir()
	startEtcd := func() (context.CancelFunc, *exec.Cmd) {
		processCtx, stop := context.WithCancel(context.Background())
		cmd := exec.CommandContext(processCtx, binary,
			"--name", "server-watch-test",
			"--data-dir", dataDir,
			"--listen-client-urls", clientURL,
			"--advertise-client-urls", clientURL,
			"--listen-peer-urls", peerURL,
			"--initial-advertise-peer-urls", peerURL,
			"--initial-cluster", "server-watch-test="+peerURL,
			"--logger", "zap", "--log-level", "error")
		if err := cmd.Start(); err != nil {
			stop()
			t.Fatal(err)
		}
		return stop, cmd
	}
	stopProcess, cmd := startEtcd()
	t.Cleanup(func() {
		stopProcess()
		_ = cmd.Wait()
	})
	cli, err := etcd.New(etcd.Config{Endpoints: []string{clientURL}, DialTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	waitReady := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for {
			getCtx, getCancel := context.WithTimeout(ctx, 250*time.Millisecond)
			_, err = cli.Get(getCtx, "ready")
			getCancel()
			if err == nil {
				return
			}
			if ctx.Err() != nil {
				t.Fatalf("etcd did not start: %v", err)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	waitReady()

	watchCtx, stopWatch := context.WithCancel(context.Background())
	notifications := make(chan struct{}, 1)
	done := make(chan struct{})
	controllerStop := make(chan struct{})
	controllerDone := make(chan struct{})
	reconciled := make(chan struct{}, 2)
	go func() {
		defer close(done)
		watchServerChanges(watchCtx, cli.Watch, notifications, 10*time.Millisecond)
	}()
	go func() {
		defer close(controllerDone)
		runServerController(controllerStop, notifications, 5*time.Second, 100*time.Millisecond, func() {
			reconciled <- struct{}{}
		})
	}()
	defer func() {
		close(controllerStop)
		stopWatch()
		waitServerSignal(t, done, time.Second)
		waitServerSignal(t, controllerDone, time.Second)
	}()
	waitServerSignal(t, reconciled, 3*time.Second)
	putCtx, cancelPut := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = cli.Put(putCtx, db.ServerPrefix+"/watch-test", "before-restart")
	cancelPut()
	if err != nil {
		t.Fatal(err)
	}
	waitServerSignal(t, reconciled, 2*time.Second)

	stopProcess()
	_ = cmd.Wait()
	stopProcess, cmd = startEtcd()
	waitReady()
	select {
	case <-reconciled:
	default:
	}
	putCtx, cancelPut = context.WithTimeout(context.Background(), 3*time.Second)
	_, err = cli.Put(putCtx, db.ServerPrefix+"/watch-test", "after-restart")
	cancelPut()
	if err != nil {
		t.Fatal(err)
	}
	waitServerSignal(t, reconciled, 2*time.Second)
}

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
			Entry("ssh key fetch transport error is retryable", errors.New("failed to fetch keys from https://github.com/foo.keys: Get \"https://github.com/foo.keys\": dial tcp: i/o timeout"), true),
			Entry("ssh key fetch http status error is retryable", errors.New("unexpected HTTP status 503 from https://github.com/foo.keys"), true),
			Entry("ssh key empty response is retryable", errors.New("no public keys found at https://github.com/foo.keys"), true),
			Entry("missing qcow2 source file copy is retryable", errors.New("failed to copy QCOW2 volume from /var/lib/marmot/images/x/osimage-x.qcow2 to /var/lib/marmot/volumes/boot-y.qcow2: exit status 1 (output: cp: cannot stat '/var/lib/marmot/images/x/osimage-x.qcow2': No such file or directory)"), true),
			Entry("other provisioning error is not retryable", errors.New("boot volume path is required for qcow2"), false),
			Entry("nil error is not retryable", nil, false),
		)
	})
})
