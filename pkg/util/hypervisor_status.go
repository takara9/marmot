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
	"log/slog"
	"net/http"

	"io"
	"time"

	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
	etcd "go.etcd.io/etcd/client/v3"
)

// ハイパーバイザーのリストを取り出す
func getHypervisors(dbUrl string) ([]db.Hypervisor, error) {
	Conn, err := db.Connect(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return nil, err
	}
	// クローズが無い？

	// ハイパーバイザーのリストを取り出す
	resp, err := db.GetEtcdByPrefix(Conn, "hv")
	if err != nil {
		slog.Error("", "err", err)
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
		slog.Error("", "err", err)
		return nil, err
	}
	// クローズが無い？

	hvs, err := getHypervisors(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return nil, err
	}

	// 自ノードを含むハイパーバイザーの死活チェック、DBへ反映
	for _, val := range hvs {
		// ハイパーバイザーの状態をDBへ書き込み
		err = db.PutDataEtcd(Conn, val.Key, val)
		if err != nil {
			slog.Error("", "err", err)
		}
	}
	return hvs, nil
}

// 短いタイムアウトで、死活監視用
func ReqGetQuick(apipath string, api string) (*http.Response, []byte, error) {

	// タイムアウト設定
	client := http.Client{
		Timeout: 2 * time.Second,
	}
	// HTTP GET
	xxx := fmt.Sprintf("%s/%s", api, apipath)
	fmt.Println("xxxxxxxxxxxxxxxxxxxx = ", xxx)

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
		slog.Error("", "err", err)
		return err
	}
	// クローズが無い？

	hv, err := db.GetHvByKey(Conn, node)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	for i := 0; i < len(hv.StgPool); i++ {
		total_sz, free_sz, err := lvm.CheckVG(hv.StgPool[i].VolGroup)
		if err != nil {
			slog.Error("", "err", err)
			return err
		}
		hv.StgPool[i].FreeCap = free_sz / 1024 / 1024 / 1024
		hv.StgPool[i].VgCap = total_sz / 1024 / 1024 / 1024
	}

	// DBへ書き込み
	err = db.PutDataEtcd(Conn, hv.Key, hv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	return nil

}

// ボリュームグループの容量を取得して、DBへセットする
func CheckHvVG(dbUrl string, node string, vg string) error {

	// DBへのアクセス
	Conn, err := db.Connect(dbUrl)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	err = CheckHvVG2(Conn, node, vg)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	return nil
}

// ボリュームグループの容量を取得して、DBへセットする
func CheckHvVG2(Conn *etcd.Client, node string, vg string) error {

	// LVMへのアクセス
	total_sz, free_sz, err := lvm.CheckVG(vg)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// キーを取得
	hv, err := db.GetHvByKey(Conn, node)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 一致するVGにデータをセット
	for i := 0; i < len(hv.StgPool); i++ {
		if hv.StgPool[i].VolGroup == vg {
			hv.StgPool[i].FreeCap = free_sz / 1024 / 1024 / 1024
			hv.StgPool[i].VgCap = total_sz / 1024 / 1024 / 1024
		}

	}

	// DBへ書き込み
	err = db.PutDataEtcd(Conn, hv.Key, hv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	return nil
}
