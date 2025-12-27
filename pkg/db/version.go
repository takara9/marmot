package db

import (
	"log/slog"

	"github.com/takara9/marmot/api"
)

func (d *Database) SetVersion(ver api.Version) error {
	slog.Debug("Called SetVersion with open api")

	lockKey := "/lock/version"
	mutex, err := d.LockKey(lockKey)
	if err != nil {
		slog.Error("LockKey()", "err", err, "key", lockKey)
		return err
	}
	defer d.UnlockKey(mutex)

	if err := d.PutJSON(VersionKey, ver); err != nil {
		slog.Error("failed to write database", "err", err, "version", ver)
		return err
	}

	return nil
}

func (d *Database) GetVersion() (*api.Version, error) {
	var v api.Version

	_, err := d.GetJSON(VersionKey, &v)
	if err != nil {
		slog.Error("failed to read from database", "err", err)
		return nil, err
	}

	return &v, nil
}
