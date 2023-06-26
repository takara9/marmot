package main

import (
	"fmt"
	"log"
	db "github.com/takara9/marmot/pkg/db"
	etcd "go.etcd.io/etcd/client/v3"
)


// ハイパーバイザーの設定
func SetHypervisor(con *etcd.Client, v Hypervisor) error {

	var hv db.Hypervisor
	hv.Nodename    = v.Name
	hv.Key         = v.Name             // Key
	hv.IpAddr      = v.IpAddr
	hv.Cpu         = int(v.Cpu)
	hv.FreeCpu     = int(v.Cpu)         // これで良いのか
	hv.Memory      = int(v.Ram * 1024)  // MB
	hv.FreeMemory  = int(v.Ram * 1024)
	hv.Status      = 0

	for _, val := range v.Storage {
		var sp db.StoragePool
		sp.VolGroup = val.VolGroup
		sp.Type     = val.Type
		hv.StgPool = append(hv.StgPool,sp)
	}

	err := db.PutDataEtcd(con, hv.Key , hv)
	if err != nil {
		log.Println("db.PutDataEtcd()", err)
		return err
	}
	return nil

}

// イメージテンプレート
func SetImageTemplate(con *etcd.Client,v Image) error {
	var osi db.OsImageTemplate
	osi.LogicaVol   = v.LogicalVolume
	osi.VolumeGroup = v.VolumeGroup
	osi.OsVariant   = v.Name
	key := fmt.Sprintf("%v_%v", "OSI", osi.OsVariant)
	err := db.PutDataEtcd(con, key, osi)
	if err != nil {
		log.Println("db.PutDataEtcd()", err)
		return err
	}
	return nil
}



func main() {

	Conn,err := db.Connect("http://hv3:2379")
	if err != nil {
		panic(err)
	}
	defer Conn.Close()

	var fn string = "hypervisor-config.yaml"
	var hvs Hypervisors
	readYAML(fn, &hvs)

	// ハイパーバイザー
	for _, hv := range hvs.Hvs {
		fmt.Println(hv)
		SetHypervisor(Conn, hv)
	}

	// OSイメージテンプレート
	for _, hd := range hvs.Imgs {
		SetImageTemplate(Conn, hd)
	}

	// シーケンス番号のリセット
	for _, sq := range hvs.Seq {
		db.CreateSeq(Conn, sq.Key, sq.Start, sq.Step)
	}

}
