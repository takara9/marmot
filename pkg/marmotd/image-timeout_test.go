package marmotd

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("Image timeout helpers", func() {
	Describe("contextTimeoutHint", func() {
		It("returns zero for nil context", func() {
			Expect(contextTimeoutHint(nil)).To(BeZero())
		})

		It("returns zero when deadline is not set", func() {
			Expect(contextTimeoutHint(context.Background())).To(BeZero())
		})

		It("returns a rounded timeout when deadline is set", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2200*time.Millisecond)
			defer cancel()

			hint := contextTimeoutHint(ctx)
			Expect(hint).To(BeNumerically(">=", 2*time.Second))
			Expect(hint).To(BeNumerically("<=", 3*time.Second))
		})

		It("returns zero for expired deadlines", func() {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()

			Expect(contextTimeoutHint(ctx)).To(BeZero())
		})
	})

	Describe("wrapDeadlineExceeded", func() {
		It("returns nil when error is nil", func() {
			Expect(wrapDeadlineExceeded(nil, "op", time.Second)).To(BeNil())
		})

		It("returns the original error when it is not a deadline error", func() {
			baseErr := errors.New("boom")

			Expect(wrapDeadlineExceeded(baseErr, "op", time.Second)).To(MatchError(baseErr))
		})

		It("wraps deadline exceeded with operation and timeout", func() {
			err := wrapDeadlineExceeded(context.DeadlineExceeded, "イメージ作成", 5*time.Minute)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("イメージ作成がタイムアウトしました"))
			Expect(err.Error()).To(ContainSubstring("timeout=5m0s"))
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
		})

		It("wraps deadline exceeded without timeout text when timeout is zero", func() {
			err := wrapDeadlineExceeded(context.DeadlineExceeded, "ダウンロード", 0)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ダウンロードがタイムアウトしました"))
			Expect(err.Error()).NotTo(ContainSubstring("timeout="))
		})
	})

	Describe("newTimeoutContext", func() {
		It("creates a cancellable context without deadline when timeout is non-positive", func() {
			ctx, cancel := newTimeoutContext(nil, 0)
			defer cancel()

			_, ok := ctx.Deadline()
			Expect(ok).To(BeFalse())
		})

		It("creates a context with deadline when timeout is positive", func() {
			ctx, cancel := newTimeoutContext(context.Background(), time.Second)
			defer cancel()

			deadline, ok := ctx.Deadline()
			Expect(ok).To(BeTrue())
			Expect(time.Until(deadline)).To(BeNumerically(">", 0))
		})
	})

	Describe("markImageCreationFailed", func() {
		var (
			originalUpdateImageRecord        func(*Marmot, api.Image) error
			originalUpdateImageFailureStatus func(*Marmot, string, string)
		)

		BeforeEach(func() {
			originalUpdateImageRecord = updateImageRecord
			originalUpdateImageFailureStatus = updateImageFailureStatus
		})

		AfterEach(func() {
			updateImageRecord = originalUpdateImageRecord
			updateImageFailureStatus = originalUpdateImageFailureStatus
		})

		It("returns nil when error is nil", func() {
			m := &Marmot{}
			image := api.Image{Id: "img-1"}

			Expect(m.markImageCreationFailed(image, nil)).To(BeNil())
		})

		It("updates the image object when status exists", func() {
			m := &Marmot{}
			image := api.Image{
				Id: "img-2",
				Status: &api.Status{
					StatusCode: db.IMAGE_CREATING,
				},
			}
			baseErr := errors.New("copy failed")

			var updated api.Image
			updateImageRecord = func(_ *Marmot, in api.Image) error {
				updated = in
				return nil
			}
			updateImageFailureStatus = func(_ *Marmot, imageID, message string) {
				Fail("fallback status update should not be called")
			}

			err := m.markImageCreationFailed(image, baseErr)

			Expect(err).To(MatchError(baseErr))
			Expect(updated.Id).To(Equal("img-2"))
			Expect(updated.Status).NotTo(BeNil())
			Expect(updated.Status.StatusCode).To(Equal(db.IMAGE_CREATION_FAILED))
			Expect(updated.Status.Status).NotTo(BeNil())
			Expect(*updated.Status.Status).To(Equal(db.ImageStatus[db.IMAGE_CREATION_FAILED]))
			Expect(updated.Status.Message).NotTo(BeNil())
			Expect(*updated.Status.Message).To(Equal("copy failed"))
			Expect(updated.Status.LastUpdateTimeStamp).NotTo(BeNil())
		})

		It("falls back to status message update when status is nil", func() {
			m := &Marmot{}
			image := api.Image{Id: "img-3"}
			baseErr := errors.New("timeout")

			called := false
			updateImageRecord = func(_ *Marmot, in api.Image) error {
				Fail("direct image update should not be called")
				return nil
			}
			updateImageFailureStatus = func(_ *Marmot, imageID, message string) {
				called = true
				Expect(imageID).To(Equal("img-3"))
				Expect(message).To(Equal("timeout"))
			}

			err := m.markImageCreationFailed(image, baseErr)

			Expect(err).To(MatchError(baseErr))
			Expect(called).To(BeTrue())
		})

		It("keeps returning the original error even if image update fails", func() {
			m := &Marmot{}
			image := api.Image{
				Id:     "img-4",
				Status: &api.Status{},
			}
			baseErr := errors.New("write failed")
			updateImageRecord = func(_ *Marmot, in api.Image) error {
				return errors.New("db unavailable")
			}

			Expect(m.markImageCreationFailed(image, baseErr)).To(MatchError(baseErr))
		})
	})

	Describe("integration of timeout message helpers", func() {
		It("can build a failed image message from a deadline error", func() {
			m := &Marmot{}
			image := api.Image{
				Id:     "img-5",
				Status: &api.Status{},
			}

			var updated api.Image
			updateImageRecord = func(_ *Marmot, in api.Image) error {
				updated = in
				return nil
			}

			err := wrapDeadlineExceeded(context.DeadlineExceeded, "実行中 VM からのイメージ作成", 10*time.Minute)
			Expect(m.markImageCreationFailed(image, err)).To(HaveOccurred())
			Expect(updated.Status).NotTo(BeNil())
			Expect(updated.Status.Message).NotTo(BeNil())
			Expect(*updated.Status.Message).To(ContainSubstring("実行中 VM からのイメージ作成がタイムアウトしました"))
		})
	})
})
