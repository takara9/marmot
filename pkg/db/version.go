package db

import (
	"log/slog"

	"github.com/takara9/marmot/api"
)

func (d *Database) SetVersion(ver api.Version) error {
	slog.Debug("Called SetVersion with open api")
	mutex, err := d.LockKey(VersionKey)
	if err != nil {
		slog.Error("LockKey()", "err", err, "key", VersionKey)
		return err
	}
	defer d.UnlockKey(mutex)

	if err := d.PutJSON(VersionKey, ver); err != nil {
		slog.Error("PutJSON()", "err", err, "version", ver)
		return err
	}

	return nil
}

func (d *Database) GetVersion() (*api.Version, error) {
	slog.Debug("Called GetVersion with open api")
	var v api.Version
	_, err := d.GetJSON(VersionKey, &v)
	if err != nil {
		slog.Error("GetJSON()", "err", err)
		return nil, err
	}
	return &v, nil
}
