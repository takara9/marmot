package util

/*
   ハイパーバイザーの状態取得して、DBへ反映させる
*/

// データベースからHVのリストを取り出して、
// RESTレベルの死活チェック、PVSのチェック、ストレージ空き容量チェックを実行する
// チェック結果をetcdへ反映させる

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	//"strings"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	etcd "go.etcd.io/etcd/client/v3"
	"io"
	"time"
)

// ハイパーバイザーのリストを取り出す
func getHypervisors(dbUrl string) ([]db.Hypervisor, error) {
	Conn, err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect()", " ", err)
		return nil, err
	}
	// クローズが無い？

	// ハイパーバイザーのリストを取り出す
	resp, err := db.GetEtcdByPrefix(Conn, "hv")
	if err != nil {
		log.Println("GetEtcdByPrefix()", err)
		return nil, err
	}

	var hvs []db.Hypervisor
	for _, val := range resp.Kvs {
		var hv db.Hypervisor
		err = json.Unmarshal(val.Value, &hv)
		if err != nil {
			return nil, err
		}
		hvs = append(hvs, hv)
	}
	return hvs, nil
}

// ハイパーバイザーをREST-APIでアクセスして疎通を確認、DBへ反映させる
func CheckHypervisors(dbUrl string, node string) ([]db.Hypervisor, error) {
	Conn, err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect()", " ", err)
		return nil, err
	}
	// クローズが無い？

	hvs, err := getHypervisors(dbUrl)
	if err != nil {
		log.Println("GetHypervisors()", err)
		return nil, err
	}

	// 自ノードを含むハイパーバイザーの死活チェック、DBへ反映
	for _, val := range hvs {

		// ping を変更してデータを収集も可能
		/*
			_, body, err := ReqGetQuick("ping", fmt.Sprintf("http://%s:8750",val.IpAddr))
			if err != nil {
				log.Println("Communication failed", err, string(body))
				val.Status = 0
			} else {
				dec := json.NewDecoder(strings.NewReader(string(body)))
				var msg message
				dec.Decode(&msg)
				if msg.Message == "ok" {
					val.Status = 2 // 正常稼働中
				} else {
					val.Status = 1 // 異常発生中
				}
			}
		*/

		// ハイパーバイザーの状態をDBへ書き込み
		err = db.PutDataEtcd(Conn, val.Key, val)
		if err != nil {
			log.Println("db.PutDataEtcd()", err)
		}
	}
	return hvs, nil
}

// ping の結果を受け取るための構造体、暫定的に配置
//type message struct {
//	Message string
//}

// 短いタイムアウトで、死活監視用
func ReqGetQuick(apipath string, api string) (*http.Response, []byte, error) {

	// タイムアウト設定
	client := http.Client{
		Timeout: 2 * time.Second,
	}
	// HTTP GET
	xxx := fmt.Sprintf("%s/%s", api, apipath)
	fmt.Println("xxxxxxxxxxxxxxxxxxxx = ", xxx)

	//res, err := client.Get(fmt.Sprintf("%s/%s", api, apipath))
	//if err != nil {
	//	return nil, nil, err
	//}

	res, err := client.Get(fmt.Sprintf("%s/%s", api, apipath))
	if err != nil {
		log.Printf("--------%v------------ %v", res, err)
		return nil, nil, err
	}
	defer res.Body.Close()
	// READ REPLY
	byteBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, nil, err
	}
	return res, byteBody, nil
}

func CheckHvVgAll(dbUrl string, node string) error {
	Conn, err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect()", " ", err)
		return err
	}
	// クローズが無い？

	hv, err := db.GetHvByKey(Conn, node)
	if err != nil {
		log.Println("db.GetHvByKey()", err)
		return err
	}

	for i := 0; i < len(hv.StgPool); i++ {
		total_sz, free_sz, err := lvm.CheckVG(hv.StgPool[i].VolGroup)
		if err != nil {
			log.Println("LVM Operation Failed", err)
			return err
		}
		hv.StgPool[i].FreeCap = free_sz / 1024 / 1024 / 1024
		hv.StgPool[i].VgCap = total_sz / 1024 / 1024 / 1024
	}

	// DBへ書き込み
	err = db.PutDataEtcd(Conn, hv.Key, hv)
	if err != nil {
		log.Println("db.PutDataEtcd()", err)
		return err
	}
	return nil

}

// ボリュームグループの容量を取得して、DBへセットする
func CheckHvVG(dbUrl string, node string, vg string) error {

	// DBへのアクセス
	Conn, err := db.Connect(dbUrl)
	if err != nil {
		log.Println("db.Connect()", err)
		return err
	}

	err = CheckHvVG2(Conn, node, vg)
	if err != nil {
		log.Println("CheckHvVG2()", err)
		return err
	}

	return nil
}

// ボリュームグループの容量を取得して、DBへセットする
func CheckHvVG2(Conn *etcd.Client, node string, vg string) error {

	// LVMへのアクセス
	total_sz, free_sz, err := lvm.CheckVG(vg)
	if err != nil {
		log.Println("LVM Operation Failed", err)
		return err
	}

	// キーを取得
	hv, err := db.GetHvByKey(Conn, node)
	if err != nil {
		log.Println("db.GetHvByKey()", err)
		return err
	}

	// 一致するVGにデータをセット
	for i := 0; i < len(hv.StgPool); i++ {
		if hv.StgPool[i].VolGroup == vg {
			hv.StgPool[i].FreeCap = free_sz / 1024 / 1024 / 1024
			hv.StgPool[i].VgCap = total_sz / 1024 / 1024 / 1024
		}

	}

	/* 注意 この書き方では、データがセットされない
	for _,v := range hv.StgPool {
		if v.VolGroup == vg {
			v.FreeCap = free_sz/1024/1024/1024
			v.VgCap   = total_sz/1024/1024/1024
		}
	}
	*/

	// DBへ書き込み
	err = db.PutDataEtcd(Conn, hv.Key, hv)
	if err != nil {
		log.Println("db.PutDataEtcd()", err)
		return err
	}

	return nil
}
