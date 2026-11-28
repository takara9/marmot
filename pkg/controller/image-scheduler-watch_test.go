package controller

import (
	"context"
	"net"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/db"
	etcd "go.etcd.io/etcd/client/v3"
)

var _ = Describe("image and scheduler controller watch", func() {
	It("reconciles only on the keys each controller owns", func() {
		binary, err := exec.LookPath("etcd")
		if err != nil {
			Skip("etcd binary not available")
		}
		listener := func() string {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).NotTo(HaveOccurred())
			address := l.Addr().String()
			Expect(l.Close()).To(Succeed())
			return "http://" + address
		}
		clientURL, peerURL := listener(), listener()
		processCtx, stopProcess := context.WithCancel(context.Background())
		cmd := exec.CommandContext(processCtx, binary,
			"--name", "image-scheduler-watch-test", "--data-dir", GinkgoT().TempDir(),
			"--listen-client-urls", clientURL, "--advertise-client-urls", clientURL,
			"--listen-peer-urls", peerURL, "--initial-advertise-peer-urls", peerURL,
			"--initial-cluster", "image-scheduler-watch-test="+peerURL,
			"--logger", "zap", "--log-level", "error")
		Expect(cmd.Start()).To(Succeed())
		DeferCleanup(func() {
			stopProcess()
			_ = cmd.Wait()
		})
		cli, err := etcd.New(etcd.Config{Endpoints: []string{clientURL}, DialTimeout: time.Second})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = cli.Close() })
		Eventually(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			_, err := cli.Get(ctx, "ready")
			return err
		}).WithTimeout(10 * time.Second).WithPolling(50 * time.Millisecond).Should(Succeed())

		put := func(key string) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := cli.Put(ctx, key, "value")
			Expect(err).NotTo(HaveOccurred())
		}
		startController := func(prefix, resource string, minInterval time.Duration) <-chan struct{} {
			watchCtx, stopWatch := context.WithCancel(context.Background())
			notifications := make(chan struct{}, 1)
			reconciled := make(chan struct{}, 8)
			stop := make(chan struct{})
			watchDone := make(chan struct{})
			done := make(chan struct{})
			go func() {
				defer close(watchDone)
				watchResourceChanges(watchCtx, cli.Watch, prefix, resource, notifications, time.Second)
			}()
			go func() {
				defer close(done)
				runServerController(stop, notifications, 30*time.Second, minInterval, func() { reconciled <- struct{}{} })
			}()
			DeferCleanup(func() {
				close(stop)
				stopWatch()
				Eventually(watchDone).WithTimeout(time.Second).Should(BeClosed())
				Eventually(done).WithTimeout(time.Second).Should(BeClosed())
			})
			Eventually(reconciled).WithTimeout(3 * time.Second).Should(Receive())
			return reconciled
		}

		By("the image controller ignores the OS template image keys")
		image := startController(db.ImagePrefix, "Image", imageWatchMinInterval)
		put(db.OsTemplateImagePrefix + "/watch-test")
		Consistently(image).WithTimeout(500 * time.Millisecond).ShouldNot(Receive())
		put(db.ImagePrefix + "/watch-test")
		Eventually(image).WithTimeout(2 * time.Second).Should(Receive())

		By("the scheduler controller ignores the host status heartbeat")
		scheduler := startController(db.ServerPrefix, "Scheduler", schedulerWatchMinInterval)
		put(db.HostStatusPrefix + "/watch-test")
		Consistently(scheduler).WithTimeout(500 * time.Millisecond).ShouldNot(Receive())
		put(db.ServerPrefix + "/watch-test")
		Eventually(scheduler).WithTimeout(2 * time.Second).Should(Receive())
	})
})
