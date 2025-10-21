package marmotd

import (
	db "github.com/takara9/marmot/pkg/db"
)

// main から　Marmot を分離する？
type Marmot struct {
	NodeName string
	EtcdUrl  string
	Db       *db.Database
}

func NewMarmot(nodeName string, etcdUrl string) (*Marmot, error) {
	var m Marmot
	var err error
	m.Db, err = db.NewDatabase(etcdUrl)
	if err != nil {
		return nil, err
	}
	m.NodeName = nodeName
	m.EtcdUrl = etcdUrl
	return &m, nil
}
