package db

import (
	"encoding/json"
	"log/slog"

	"github.com/takara9/marmot/api"
)

func (d *Database) SetVersion(ver api.Version) error {
	slog.Debug("Called SetVersion with open api")
	err := d.PutDataEtcd(VersionKey, ver)
	if err != nil {
		slog.Error("PutDataEtcd()", "err", err, "version", ver)
		return err
	}

	return nil
}

func (d *Database) GetVersion() (*api.Version, error) {
	var v api.Version

	ver, err := d.GetByKey(VersionKey)
	if err != nil {
		slog.Error("GetByKey()", "err", err)
		return nil, err
	}

	err = json.Unmarshal(ver, &v)
	if err != nil {
		slog.Error("json.Unmarshal()", "err", err, VersionKey, ver)
		return nil, err
	}

	return &v, nil
}
