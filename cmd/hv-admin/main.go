package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	db "github.com/takara9/marmot/pkg/db"
)

var node *string
var etcd *string

type Marmotd struct {
	dbc      db.Db
	NodeName string
}

func NewMarmotd(etcdUrl string, nodeName string) (Marmotd, error) {
	var m Marmotd
	dbc, err := db.NewEtcdEp(etcdUrl)
	if err != nil {
		slog.Error("faild to create endpoint of etcd database", "err", err)
		os.Exit(1)
	}
	m.dbc = dbc
	return m, nil
}

func main() {
	node = flag.String("node", "hv1", "Hypervisor node name")
	etcd = flag.String("etcd", "http://127.0.0.1:2379", "etcd url")
	flag.Parse()

	m, err := NewMarmotd(*etcd, *node)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	hvs, cnf, err := m.ReadHvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	err = m.SetHvConfig(hvs, cnf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
