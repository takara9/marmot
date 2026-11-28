package controller

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
	etcd "go.etcd.io/etcd/client/v3"
)

var _ = Describe("volume controller watch", func() {
	It("restarts a compacted watch on the volume prefix and notifies", func() {
		ctx, cancel := context.WithCancel(context.Background())
		notifications := make(chan struct{}, 1)
		keys := make(chan string, 2)
		attempts := 0
		watch := func(ctx context.Context, key string, _ ...etcd.OpOption) etcd.WatchChan {
			keys <- key
			attempts++
			ch := make(chan etcd.WatchResponse, 1)
			if attempts == 1 {
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
			watchResourceChanges(ctx, watch, db.VolumePrefix, "Volume", notifications, 10*time.Millisecond)
		}()
		DeferCleanup(func() {
			cancel()
			Eventually(done).WithTimeout(time.Second).Should(BeClosed())
		})

		Eventually(keys).WithTimeout(time.Second).Should(Receive(Equal(db.VolumePrefix + "/")))
		Eventually(keys).WithTimeout(time.Second).Should(Receive(Equal(db.VolumePrefix + "/")))
		Eventually(notifications).WithTimeout(time.Second).Should(Receive())
	})

	It("reconciles volume changes before the periodic tick and recovers after etcd restarts", func() {
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
				"--name", "volume-watch-test", "--data-dir", dataDir,
				"--listen-client-urls", clientURL, "--advertise-client-urls", clientURL,
				"--listen-peer-urls", peerURL, "--initial-advertise-peer-urls", peerURL,
				"--initial-cluster", "volume-watch-test="+peerURL,
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
		reconciled := make(chan string, 8)
		go func() {
			defer close(watchDone)
			watchResourceChanges(watchCtx, cli.Watch, db.VolumePrefix, "Volume", notifications, 10*time.Millisecond)
		}()
		go func() {
			defer close(done)
			runServerController(stop, notifications, 30*time.Second, volumeWatchMinInterval, func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				resp, err := cli.Get(ctx, db.VolumePrefix+"/watch-test")
				if err != nil {
					reconciled <- "read error: " + err.Error()
				} else if len(resp.Kvs) == 0 {
					reconciled <- ""
				} else {
					reconciled <- string(resp.Kvs[0].Value)
				}
			})
		}()
		DeferCleanup(func() {
			close(stop)
			stopWatch()
			Eventually(watchDone).WithTimeout(time.Second).Should(BeClosed())
			Eventually(done).WithTimeout(time.Second).Should(BeClosed())
		})

		By("reconciling when the watch starts")
		Eventually(reconciled).WithTimeout(3 * time.Second).Should(Receive(Equal("")))
		By("ignoring changes outside the volume prefix")
		put(db.ServerPrefix+"/watch-test", "unrelated")
		Consistently(reconciled).WithTimeout(250 * time.Millisecond).ShouldNot(Receive())
		By("reconciling a volume update without waiting for the five-second tick")
		put(db.VolumePrefix+"/watch-test", "before-restart")
		Eventually(reconciled).WithTimeout(2 * time.Second).Should(Receive(Equal("before-restart")))
		By("rate limiting another update using the production interval")
		put(db.VolumePrefix+"/watch-test", "second-update")
		Consistently(reconciled).WithTimeout(700 * time.Millisecond).ShouldNot(Receive())
		Eventually(reconciled).WithTimeout(2 * time.Second).Should(Receive(Equal("second-update")))
		By("recovering after etcd restarts")
		stopProcess()
		_ = cmd.Wait()
		stopProcess, cmd = startEtcd()
		waitReady()
		select {
		case <-reconciled:
		default:
		}
		put(db.VolumePrefix+"/watch-test", "after-restart")
		Eventually(reconciled).WithTimeout(2 * time.Second).Should(Receive(Equal("after-restart")))
	})
})

func TestShouldDeleteVolumeForMissingAssignedNode(t *testing.T) {
	hvc := util.StringPtr("hvc")
	ws1 := util.StringPtr("ws1")
	statuses := []api.HostStatus{{NodeName: hvc}}

	tests := []struct {
		name       string
		vol        api.Volume
		statuses   []api.HostStatus
		wantDelete bool
		wantReason string
	}{
		{
			name: "pending volume with missing assigned node does not delete",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_PENDING},
			},
			statuses:   statuses,
			wantDelete: false,
			wantReason: "",
		},
		{
			name: "deleting volume with existing assigned node does not delete",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: hvc},
				Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
			},
			statuses:   statuses,
			wantDelete: false,
			wantReason: "",
		},
		{
			name: "deleting volume with missing assigned node deletes",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
			},
			statuses:   statuses,
			wantDelete: true,
			wantReason: "assigned_node_not_found",
		},
		{
			name: "empty cluster status skips delete to avoid false positives",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
			},
			statuses:   []api.HostStatus{},
			wantDelete: false,
			wantReason: "",
		},
		{
			name: "nil nodeName does not delete",
			vol: api.Volume{
				Metadata: api.Metadata{},
				Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
			},
			statuses:   statuses,
			wantDelete: false,
			wantReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDelete, gotReason := shouldDeleteVolumeForMissingAssignedNode(tt.vol, tt.statuses)
			if gotDelete != tt.wantDelete {
				t.Fatalf("shouldDeleteVolumeForMissingAssignedNode() = %v, want %v", gotDelete, tt.wantDelete)
			}
			if gotReason != tt.wantReason {
				t.Fatalf("shouldDeleteVolumeForMissingAssignedNode() reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

func TestShouldFailVolumeForMissingAssignedNode(t *testing.T) {
	hvc := util.StringPtr("hvc")
	ws1 := util.StringPtr("ws1")
	statuses := []api.HostStatus{{NodeName: hvc}}

	tests := []struct {
		name       string
		vol        api.Volume
		statuses   []api.HostStatus
		wantFail bool
		wantMessage string
	}{
		{
			name: "pending volume with missing assigned node fails",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_PENDING},
			},
			statuses:    statuses,
			wantFail:    true,
			wantMessage: "metadata.nodeName \"ws1\" is not found in cluster",
		},
		{
			name: "provisioning volume with missing assigned node fails",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_PROVISIONING},
			},
			statuses:    statuses,
			wantFail:    true,
			wantMessage: "metadata.nodeName \"ws1\" is not found in cluster",
		},
		{
			name: "deleting volume with missing assigned node does not fail",
			vol: api.Volume{
				Metadata: api.Metadata{NodeName: ws1},
				Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
			},
			statuses:    statuses,
			wantFail:    false,
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFail, gotMessage := shouldFailVolumeForMissingAssignedNode(tt.vol, tt.statuses)
			if gotFail != tt.wantFail {
				t.Fatalf("shouldFailVolumeForMissingAssignedNode() = %v, want %v", gotFail, tt.wantFail)
			}
			if gotMessage != tt.wantMessage {
				t.Fatalf("shouldFailVolumeForMissingAssignedNode() message = %q, want %q", gotMessage, tt.wantMessage)
			}
		})
	}
}

func TestDeletingVolumeIsNotSkippedByNodeAssignmentCheck(t *testing.T) {
	ws1 := util.StringPtr("ws1")
	statuses := []api.HostStatus{{NodeName: util.StringPtr("hvc")}}
	vol := api.Volume{
		Metadata: api.Metadata{NodeName: ws1},
		Status:   &api.Status{StatusCode: db.VOLUME_DELETING},
	}

	if shouldFail, _ := shouldFailVolumeForMissingAssignedNode(vol, statuses); shouldFail {
		t.Fatal("shouldFailVolumeForMissingAssignedNode() = true, want false for deleting volume")
	}
	if shouldDelete, reason := shouldDeleteVolumeForMissingAssignedNode(vol, statuses); !shouldDelete || reason != "assigned_node_not_found" {
		t.Fatalf("shouldDeleteVolumeForMissingAssignedNode() = (%v, %q), want (true, \"assigned_node_not_found\")", shouldDelete, reason)
	}
}

var _ = Describe("volume controller status write failures", func() {
	var (
		c        *volumeController
		statuses []int
		messages []int
		created  int
		iscsi    int
		ops      volumeProvisionOps
		writeErr error
		// setStatus の呼び出しごとの結果。空の場合は常に成功する。
		setStatusResults []error
	)
	pendingVolume := api.Volume{
		Metadata: api.Metadata{Id: "vol01"},
		Status:   &api.Status{StatusCode: db.VOLUME_PENDING},
	}
	iscsiVolume := api.Volume{
		Metadata: api.Metadata{Id: "vol02"},
		Spec: api.VolSpec{
			Type:  util.StringPtr("lvm"),
			Kind:  util.StringPtr("data"),
			Iscsi: util.BoolPtr(true),
		},
		Status: &api.Status{StatusCode: db.VOLUME_PENDING},
	}

	BeforeEach(func() {
		c = &volumeController{unrecordedVolumes: map[string]struct{}{}}
		statuses, messages = nil, nil
		created, iscsi = 0, 0
		writeErr = errors.New("etcd unavailable")
		setStatusResults = nil
		ops = volumeProvisionOps{
			setStatus: func(_ string, status int) error {
				statuses = append(statuses, status)
				if len(setStatusResults) >= len(statuses) {
					return setStatusResults[len(statuses)-1]
				}
				return nil
			},
			setStatusMessage: func(_ string, status int, _ string) error {
				messages = append(messages, status)
				return nil
			},
			create:         func(string) error { created++; return nil },
			configureISCSI: func(string) error { iscsi++; return nil },
		}
	})

	Describe("retryVolumeStatusWrite", func() {
		It("returns nil once the write succeeds", func() {
			calls := 0
			err := retryVolumeStatusWrite(3, time.Millisecond, func() error {
				calls++
				if calls < 3 {
					return writeErr
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(Equal(3))
		})

		It("returns the last error after all attempts fail", func() {
			calls := 0
			err := retryVolumeStatusWrite(3, time.Millisecond, func() error {
				calls++
				return writeErr
			})
			Expect(err).To(MatchError(writeErr))
			Expect(calls).To(Equal(3))
		})
	})

	Describe("provisionPendingVolume", func() {
		It("records AVAILABLE after creating the volume", func() {
			c.provisionPendingVolume(pendingVolume, ops)
			Expect(created).To(Equal(1))
			Expect(statuses).To(Equal([]int{db.VOLUME_PROVISIONING, db.VOLUME_AVAILABLE}))
			Expect(c.unrecordedVolumes).To(BeEmpty())
		})

		It("does not create the volume when PROVISIONING cannot be recorded", func() {
			setStatusResults = []error{writeErr}
			c.provisionPendingVolume(pendingVolume, ops)
			Expect(created).To(Equal(0))
			Expect(statuses).To(Equal([]int{db.VOLUME_PROVISIONING}))
			Expect(messages).To(BeEmpty())
			Expect(c.unrecordedVolumes).To(BeEmpty())
		})

		It("records ERROR and skips AVAILABLE when creation fails", func() {
			ops.create = func(string) error { created++; return errors.New("lvcreate failed") }
			c.provisionPendingVolume(pendingVolume, ops)
			Expect(created).To(Equal(1))
			Expect(messages).To(Equal([]int{db.VOLUME_ERROR}))
			Expect(statuses).To(Equal([]int{db.VOLUME_PROVISIONING}))
		})

		It("configures iSCSI only for iSCSI volumes", func() {
			c.provisionPendingVolume(pendingVolume, ops)
			Expect(iscsi).To(Equal(0))
			c.provisionPendingVolume(iscsiVolume, ops)
			Expect(iscsi).To(Equal(1))
		})

		It("records ERROR and skips AVAILABLE when iSCSI configuration fails", func() {
			ops.configureISCSI = func(string) error { iscsi++; return errors.New("targetcli failed") }
			c.provisionPendingVolume(iscsiVolume, ops)
			Expect(messages).To(Equal([]int{db.VOLUME_ERROR}))
			Expect(statuses).To(Equal([]int{db.VOLUME_PROVISIONING}))
		})

		It("keeps the created volume and retries AVAILABLE when it cannot be recorded", func() {
			setStatusResults = []error{nil, writeErr, writeErr, writeErr}
			c.provisionPendingVolume(pendingVolume, ops)
			Expect(created).To(Equal(1))
			Expect(statuses).To(HaveLen(1 + volumeStatusWriteAttempts))
			Expect(messages).To(BeEmpty())
			Expect(c.unrecordedVolumes).To(HaveKey("vol01"))
		})

		It("does not register the volume when a retry records AVAILABLE", func() {
			setStatusResults = []error{nil, writeErr, nil}
			c.provisionPendingVolume(pendingVolume, ops)
			Expect(statuses).To(Equal([]int{db.VOLUME_PROVISIONING, db.VOLUME_AVAILABLE, db.VOLUME_AVAILABLE}))
			Expect(c.unrecordedVolumes).To(BeEmpty())
		})
	})

	Describe("retryUnrecordedVolume", func() {
		provisioning := func(deletionTimeStamp *time.Time, lastUpdate time.Time) api.Volume {
			return api.Volume{
				Metadata: api.Metadata{Id: "vol01"},
				Status: &api.Status{
					StatusCode:          db.VOLUME_PROVISIONING,
					DeletionTimeStamp:   deletionTimeStamp,
					LastUpdateTimeStamp: &lastUpdate,
				},
			}
		}

		It("ignores volumes that were not registered", func() {
			Expect(c.retryUnrecordedVolume(provisioning(nil, time.Now()), ops.setStatus)).To(BeFalse())
			Expect(statuses).To(BeEmpty())
		})

		It("records AVAILABLE and unregisters the volume", func() {
			c.unrecordedVolumes["vol01"] = struct{}{}
			Expect(c.retryUnrecordedVolume(provisioning(nil, time.Now()), ops.setStatus)).To(BeTrue())
			Expect(statuses).To(Equal([]int{db.VOLUME_AVAILABLE}))
			Expect(c.unrecordedVolumes).To(BeEmpty())
		})

		It("keeps the volume registered and protected from stale deletion while the write keeps failing", func() {
			c.unrecordedVolumes["vol01"] = struct{}{}
			setStatusResults = []error{writeErr}
			staleSince := time.Now().Add(-2 * VOLUME_STALE_TIMEOUT)
			Expect(c.retryUnrecordedVolume(provisioning(nil, staleSince), ops.setStatus)).To(BeTrue())
			Expect(c.unrecordedVolumes).To(HaveKey("vol01"))
		})

		It("does not overwrite the status of a volume requested for deletion", func() {
			c.unrecordedVolumes["vol01"] = struct{}{}
			deletion := time.Now()
			Expect(c.retryUnrecordedVolume(provisioning(&deletion, time.Now()), ops.setStatus)).To(BeFalse())
			Expect(statuses).To(BeEmpty())
			Expect(c.unrecordedVolumes).To(BeEmpty())
		})

		It("does not overwrite a status that is no longer PROVISIONING", func() {
			c.unrecordedVolumes["vol01"] = struct{}{}
			vol := provisioning(nil, time.Now())
			vol.Status.StatusCode = db.VOLUME_DELETING
			Expect(c.retryUnrecordedVolume(vol, ops.setStatus)).To(BeFalse())
			Expect(statuses).To(BeEmpty())
			Expect(c.unrecordedVolumes).To(BeEmpty())
		})
	})

	Describe("forgetMissingUnrecordedVolumes", func() {
		It("drops volumes that no longer exist", func() {
			c.unrecordedVolumes["vol01"] = struct{}{}
			c.unrecordedVolumes["gone"] = struct{}{}
			c.forgetMissingUnrecordedVolumes([]api.Volume{{Metadata: api.Metadata{Id: "vol01"}}})
			Expect(c.unrecordedVolumes).To(HaveKey("vol01"))
			Expect(c.unrecordedVolumes).NotTo(HaveKey("gone"))
		})
	})
})
