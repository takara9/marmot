package util

import (
	"fmt"
	//"os"
	"io"
	"encoding/json"
	"log"
	"bytes"
	"net/http"
	"errors"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/virt"
	"github.com/takara9/marmot/pkg/lvm"
	"github.com/takara9/marmot/pkg/dns"
	etcd "go.etcd.io/etcd/client/v3"

)



// クラスタ削除
func DestroyCluster(cnf cf.MarmotConfig, dbUrl string) error {
	Conn,err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect(dbUrl)", err)
		return err
	}

	var NotFound bool = true
	for _,spec := range cnf.VMSpec {

		fmt.Println("spc.Name = ", spec.Name)
		fmt.Println("cnf.ClusterName = ", cnf.ClusterName)

		// クラスタ名とホスト名の重複チェック
		vmKey,_ := db.FindByHostAndClusteName(Conn, spec.Name, cnf.ClusterName)
		if len(vmKey) > 0 {
			NotFound = false
			spec.Key = vmKey
			vm, err := db.GetVmByKey(Conn, vmKey)
			if err != nil {
				log.Println("db.GetVmByKey()", err)
				continue
			}
			err = RemoteDestroyVM(vm.HvNode, spec)
			if err != nil {
				log.Println("RemoteCreateVM()", " ",err)
				continue
			}
		}
	}
	Conn.Close()

	if NotFound == true {
		return errors.New("NotExistVM")
	}
	return nil
}

// リモートとローカルHV上のVMを削除する
func RemoteDestroyVM(hvNode string, spec cf.VMSpec) error {

	byteJSON,_ := json.MarshalIndent(spec,"","    ")
	fmt.Println(string(byteJSON))

	// JSON形式でポストする
	reqURL := fmt.Sprintf("http://%s:8750/%s", hvNode, "destroyVm")
	request, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		log.Println("client.Do(request) ",err)
		return err
	}
	defer response.Body.Close()

	// レスポンスを取得する
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		return errors.New(string(body))
	}
	return nil
}


// VMの削除
func DestroyVM(Conn *etcd.Client, spec cf.VMSpec, hvNode string) error {

	vm, err := db.GetVmByKey(Conn, spec.Key)
	if err != nil {
		log.Println("db.GetVmByKey()", err)
	}

	// ハイパーバイザーのリソース削減保存のため値を取得
	hv,err := db.GetHvByKey(Conn, vm.HvNode)
	if err != nil {
		log.Println("db.GetHvByKey()", err)
	}

	// ステータスを調べて停止中であれば、足し算しない。
	if vm.Status != db.STOPPED || vm.Status == db.ERROR {
		hv.FreeCpu = hv.FreeCpu + vm.Cpu
		hv.FreeMemory = hv.FreeMemory + vm.Memory
		err = db.PutDataEtcd(Conn, hv.Key, hv)
		if err != nil {
			log.Println("db.PutDataEtcd()", err)
		}
	}
	// データベースから削除
	err = db.DelByKey(Conn,spec.Key)
	if err != nil {
		log.Println("db.DelByKey()", err)
	}


	// DNSから削除
	key := fmt.Sprintf("%s.%s", vm.Name, vm.ClusterName)
	err = dns.Del(dns.DnsRecord{Hostname: key},"http://ns1.labo.local:2379")
	if err != nil {
		log.Println("dns.Del()", err)
	}

	// 仮想マシンの停止＆削除
	url := "qemu:///system"
	err = virt.DestroyVM(url, spec.Key)
	if err != nil {
		log.Println("DestoryVM()", err)
	}

	// OS LVを削除
	err = lvm.RemoveLV(vm.OsVg, vm.OsLv)
	if err != nil {
		log.Println("lvm.RemoveLV()", err)
	}
	// ストレージの更新
	CheckHvVG2(Conn, hvNode, vm.OsVg)

	// データLVを削除
	for _,d := range vm.Storage {
		err = lvm.RemoveLV(d.Vg, d.Lv)
		if err != nil {
			log.Println("lvm.RemoveLV()", err)
		}
		// ストレージの更新
		CheckHvVG2(Conn, hvNode, d.Vg)
	}

	return nil
}

