package controller

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/marmotd"
	etcd "go.etcd.io/etcd/client/v3"
)

var _ = Describe("network controller watch", func() {
	It("reconciles only network key changes and recovers after etcd restarts", func() {
		binary, err := exec.LookPath("etcd")
		if err != nil {
			Skip("etcd binary not available")
		}
		freeAddress := func() string {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).NotTo(HaveOccurred())
			address := listener.Addr().String()
			Expect(listener.Close()).To(Succeed())
			return "http://" + address
		}
		clientURL, peerURL := freeAddress(), freeAddress()
		dataDir := GinkgoT().TempDir()
		startEtcd := func() (context.CancelFunc, *exec.Cmd) {
			processCtx, stop := context.WithCancel(context.Background())
			cmd := exec.CommandContext(processCtx, binary,
				"--name", "network-watch-test", "--data-dir", dataDir,
				"--listen-client-urls", clientURL, "--advertise-client-urls", clientURL,
				"--listen-peer-urls", peerURL, "--initial-advertise-peer-urls", peerURL,
				"--initial-cluster", "network-watch-test="+peerURL,
				"--logger", "zap", "--log-level", "error")
			Expect(cmd.Start()).To(Succeed())
			return stop, cmd
		}
		stopProcess, cmd := startEtcd()
		DeferCleanup(func() {
			stopProcess()
			_ = cmd.Wait()
		})
		cli, err := etcd.New(etcd.Config{Endpoints: []string{clientURL}, DialTimeout: time.Second})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = cli.Close() })
		waitReady := func() {
			Eventually(func() error {
				ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
				defer cancel()
				_, err := cli.Get(ctx, "ready")
				return err
			}).WithTimeout(10 * time.Second).WithPolling(50 * time.Millisecond).Should(Succeed())
		}
		put := func(key, value string) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := cli.Put(ctx, key, value)
			Expect(err).NotTo(HaveOccurred())
		}
		waitReady()

		watchCtx, stopWatch := context.WithCancel(context.Background())
		notifications := make(chan struct{}, 1)
		watchDone := make(chan struct{})
		stop := make(chan struct{})
		done := make(chan struct{})
		reconciled := make(chan struct{}, 8)
		go func() {
			defer close(watchDone)
			watchResourceChanges(watchCtx, cli.Watch, db.NetworkPrefix, "Network", notifications, networkWatchRetryInterval)
		}()
		go func() {
			defer close(done)
			runServerController(stop, notifications, 30*time.Second, networkWatchMinInterval, func() {
				reconciled <- struct{}{}
			})
		}()
		DeferCleanup(func() {
			close(stop)
			stopWatch()
			Eventually(watchDone).WithTimeout(time.Second).Should(BeClosed())
			Eventually(done).WithTimeout(time.Second).Should(BeClosed())
		})

		By("reconciling when the watch starts")
		Eventually(reconciled).WithTimeout(3 * time.Second).Should(Receive())
		By("ignoring the network load balancer keys that share the network prefix text")
		put(db.NetworkLoadBalancerPrefix+"/watch-test", "unrelated")
		Consistently(reconciled).WithTimeout(500 * time.Millisecond).ShouldNot(Receive())
		By("reconciling a network update without waiting for the periodic tick")
		put(db.NetworkPrefix+"/watch-test", "before-restart")
		Eventually(reconciled).WithTimeout(2 * time.Second).Should(Receive())
		By("recovering after etcd restarts")
		stopProcess()
		_ = cmd.Wait()
		stopProcess, cmd = startEtcd()
		waitReady()
		select {
		case <-reconciled:
		default:
		}
		put(db.NetworkPrefix+"/watch-test", "after-restart")
		Eventually(reconciled).WithTimeout(3 * time.Second).Should(Receive())
	})
})

var _ = Describe("network controller overlay membership reconciliation", func() {
	var (
		c     *networkController
		calls int
	)
	reconcileWith := func(changed bool, err error) func() (bool, error) {
		return func() (bool, error) {
			calls++
			return changed, err
		}
	}

	BeforeEach(func() {
		c = &networkController{marmot: &marmotd.Marmot{NodeName: "hv0"}}
		calls = 0
	})

	It("keeps the signature when the leader's reconciliation fails and retries the same change", func() {
		Expect(c.reconcileMemberChange("hv0,hv1", true, reconcileWith(false, errors.New("etcd unavailable")))).To(BeFalse())
		Expect(c.lastNetworkMemberSignature).To(BeEmpty())

		Expect(c.reconcileMemberChange("hv0,hv1", true, reconcileWith(true, nil))).To(BeTrue())
		Expect(calls).To(Equal(2))
		Expect(c.lastNetworkMemberSignature).To(Equal("hv0,hv1"))
	})

	It("reports a partial change while keeping the signature after a failed reconciliation", func() {
		Expect(c.reconcileMemberChange("hv0,hv1", true, reconcileWith(true, errors.New("one entry failed")))).To(BeTrue())
		Expect(c.lastNetworkMemberSignature).To(BeEmpty())
	})

	It("commits the signature after success and skips an unchanged membership", func() {
		Expect(c.reconcileMemberChange("hv0,hv1", true, reconcileWith(false, nil))).To(BeFalse())
		Expect(c.lastNetworkMemberSignature).To(Equal("hv0,hv1"))

		c.reconcileMemberChange("hv0,hv1", true, reconcileWith(true, nil))
		Expect(calls).To(Equal(1))
	})

	It("retries after a failed reconciliation even when the membership returns to the committed signature", func() {
		Expect(c.reconcileMemberChange("hv0", true, reconcileWith(false, nil))).To(BeFalse())
		Expect(c.reconcileMemberChange("hv0,hv1", true, reconcileWith(true, errors.New("one entry failed")))).To(BeTrue())
		Expect(c.lastNetworkMemberSignature).To(Equal("hv0"))

		Expect(c.reconcileMemberChange("hv0", true, reconcileWith(true, nil))).To(BeTrue())
		Expect(calls).To(Equal(3))
		Expect(c.reconcileMemberChange("hv0", true, reconcileWith(true, nil))).To(BeFalse())
		Expect(calls).To(Equal(3))
	})

	It("records the signature without reconciling when this node is not the leader", func() {
		Expect(c.reconcileMemberChange("hv0,hv1", false, reconcileWith(true, nil))).To(BeFalse())
		Expect(calls).To(BeZero())
		Expect(c.lastNetworkMemberSignature).To(Equal("hv0,hv1"))
	})
})
