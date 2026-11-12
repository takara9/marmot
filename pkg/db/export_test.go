package db

import "github.com/takara9/marmot/api"

func (d *Database) PutVirtualNetworkByIDForTest(rec api.VirtualNetwork) error {
	return d.putVirtualNetworkByID(rec)
}
