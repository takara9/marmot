package marmotd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

var updateImageRecord = func(m *Marmot, image api.Image) error {
	return m.Db.UpdateImage(image.Id, image)
}

var updateImageFailureStatus = func(m *Marmot, imageID, message string) {
	m.Db.UpdateImageStatusMessage(imageID, db.IMAGE_CREATION_FAILED, message)
}

func contextTimeoutHint(ctx context.Context) time.Duration {
	if ctx == nil {
		return 0
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0
	}
	timeout := time.Until(deadline)
	if timeout < 0 {
		return 0
	}
	return timeout.Round(time.Second)
}

func wrapDeadlineExceeded(err error, operation string, timeout time.Duration) error {
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if timeout > 0 {
		return fmt.Errorf("%sがタイムアウトしました (timeout=%s): %w", operation, timeout, err)
	}
	return fmt.Errorf("%sがタイムアウトしました: %w", operation, err)
}

func newTimeoutContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func (m *Marmot) markImageCreationFailed(image api.Image, err error) error {
	if err == nil {
		return nil
	}

	if image.Status != nil {
		image.Status.StatusCode = db.IMAGE_CREATION_FAILED
		image.Status.Status = util.StringPtr(db.ImageStatus[db.IMAGE_CREATION_FAILED])
		image.Status.LastUpdateTimeStamp = util.TimePtr(time.Now())
		image.Status.Message = util.StringPtr(err.Error())
		if updateErr := updateImageRecord(m, image); updateErr != nil {
			slog.Error("UpdateImage() failed while setting image failure status", "image id", image.Id, "err", updateErr)
		}
		return err
	}

	updateImageFailureStatus(m, image.Id, err.Error())
	return err
}
