package main

/*
   ハイパーバイザーの状態取得して、DBへ反映させる
*/

// データベースからHVのリストを取り出して、
// RESTレベルの死活チェック、PVSのチェック、ストレージ空き容量チェックを実行する
// チェック結果をetcdへ反映させる

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/lvm"
)

// ハイパーバイザーのリストを取り出す
func (m *Marmotd) getHypervisors() ([]db.Hypervisor, error) {
	// ハイパーバイザーのリストを取り出す
	resp, err := m.dbc.GetEtcdByPrefix("hv")
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
func (m *Marmotd) CheckHypervisors() ([]db.Hypervisor, error) {
	hvs, err := m.getHypervisors()
	if err != nil {
		slog.Error("", "err", err)
		return nil, err
	}

	// 自ノードを含むハイパーバイザーの死活チェック、DBへ反映
	for _, val := range hvs {

		// ping を変更してデータを収集も可能
		/*
			_, body, err := ReqGetQuick("ping", fmt.Sprintf("http://%s:8750",val.IpAddr))
			if err != nil {
				slog.Error("", "err", err)Communication failed", err, string(body))
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
		err = m.dbc.PutDataEtcd(val.Key, val)
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

func (m *Marmotd) CheckHvVgAll() error {
	hv, err := m.dbc.GetHvByKey(m.NodeName)
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
	err = m.dbc.PutDataEtcd(hv.Key, hv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	return nil

}

// ボリュームグループの容量を取得して、DBへセットする
func (m *Marmotd) CheckHvVG(dbUrl string, node string, vg string) error {
	err := m.CheckHvVG2(node, vg)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	return nil
}

// ボリュームグループの容量を取得して、DBへセットする
func (m *Marmotd) CheckHvVG2(node string, vg string) error {
	// LVMへのアクセス
	total_sz, free_sz, err := lvm.CheckVG(vg)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// キーを取得
	hv, err := m.dbc.GetHvByKey(node)
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

	/* 注意 この書き方では、データがセットされない
	for _,v := range hv.StgPool {
		if v.VolGroup == vg {
			v.FreeCap = free_sz/1024/1024/1024
			v.VgCap   = total_sz/1024/1024/1024
		}
	}
	*/

	// DBへ書き込み
	err = m.dbc.PutDataEtcd(hv.Key, hv)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	return nil
}
