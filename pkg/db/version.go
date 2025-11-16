package db

import (
	"log/slog"

	"github.com/takara9/marmot/api"
)

func (d *Database) SetVersion(ver api.Version) error {

	slog.Debug("Called SetVersion with open api")
	err := d.PutDataEtcd("version", ver)

	if err != nil {
		slog.Error("PutDataEtcd()", "err", err, "version", ver)
		return err
	}

	return nil
}
