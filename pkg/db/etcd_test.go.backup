package db

import (
	"fmt"
	"testing"

	etcd "go.etcd.io/etcd/client/v3"
)

//const url = "http://etcd:2379"
const url = "http://127.0.0.1:2379"

var testv = 12
var Conn *etcd.Client

// 接続テスト２ 失敗するはずだが
/*
func TestConnect_FAIL(t *testing.T) {
	url := "http://127.0.0.1:2000"
	xconn, err := Connect(url)
	if err == nil {
		t.Errorf("connect(%v), want not %v", url, err)
	}
	xconn.Close()
}
*/

// 接続テスト１
func TestConnect_SUCCESS(t *testing.T) {
	var err error
	Conn, err = Connect(url)
	if err != nil {
		t.Errorf("connect(%v), want %v", url, nil)
	}
	//
}


// HVデータの書き込み
func TestPutHVData(t *testing.T) {
	// PUT
	hv1 := testHvData1()
	fmt.Println(hv1)
	err := PutDataEtcd(Conn, hv1.Key, hv1)
	if err != nil {
		t.Errorf("putDataEtcd(%v,%v,%v) want nil %v", Conn, hv1.Key, hv1, err)
	}
}

// HVデータの取り出し
func TestGetHVData(t *testing.T) {
	// GET
	hv2, err := GetHvByKey(Conn, "hv01")
	if err != nil {
		t.Errorf("getHvByKey(%v,%v) want nil %v", Conn, "hv01", err)
	}
	if "hv01" != hv2.Key {
		t.Errorf("getHvByKey() want key %v == %v", "hv01", hv2.Key)
	}

	fmt.Println("hv key = ", hv2.Key, hv2.Nodename, hv2.Cpu)
}

// HVデータの削除
func TestDeleteHVData(t *testing.T) {
	key := "hv01"

	// DEL
	err := DelByKey(Conn, key)
	if err != nil {
		t.Errorf("delByKey(%v,%v) want nil %v", Conn, key, err)
	}

	_, err = GetHvByKey(Conn, key)
	if err != nil {
		t.Errorf("Delete %v failed ", key)
	}
}

// シーケンス番号の削除
func TestSeqnoDel(t *testing.T) {
	err := DelByKey(Conn, "serial")
	if err != nil {
		t.Errorf("delByKey()")
	}
}

// シーケンス番号の初期化と番号の取出し
func TestSeqnoInc(t *testing.T) {

	tests := []struct {
		name string
		want uint64
	}{
		{name: "inital", want: 1},
		{name: "2nd", want: 2},
		{name: "3rd", want: 3},
		{name: "4th", want: 4},
	}
	for _, tt := range tests {
		fmt.Println(tt.name, tt.want)
		/*
			t.Run(tt.name, func(t *testing.T) {
				seqno, err := GetSeq(Conn, tt.name)
				if err != nil {
					t.Errorf("getSeq(conn) return %v, want %v", seqno, tt.want)
				}
				if tt.want != seqno {
					t.Errorf("getSeq(conn) return %v, want %v", seqno, tt.want)
				}
			})
		*/
	}
}

// テストのセットアップ シーケンス番後のリセット
func TestSeqSetup(t *testing.T) {
	err := DelByKey(Conn, "serial")
	if err != nil {
		t.Errorf("delByKey()")
	}
}

// テストのセットアップ HVのセットアップ
func TestHVSetup(t *testing.T) {

	type hvReq struct {
		name string
		cpu  int
		ram  int
	}

	tests := []struct {
		name string
		req  hvReq
		want *int
	}{
		{name: "1st", req: hvReq{name: "hv1", cpu: 10, ram: 64}, want: nil},
		{name: "2nd", req: hvReq{name: "hv2", cpu: 10, ram: 64}, want: nil},
	}

	var no = 1
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hv := testHvCreate(tt.req.name, tt.req.cpu, tt.req.ram)
			err := PutDataEtcd(Conn, tt.req.name, hv)
			if err != nil {
				t.Errorf("putDataEtcd(%v,%v,%v) want nil %v", Conn, tt.req.name, hv, err)
			}
		})
		no = no + 1
	}
}

// VMスケジューリングのテスト
// func assignHvforVm(con *etcd.Client, vm VirtualMachine) (string, error) {
/*
   分散スケジューリングのモジュールが未実装のため
   一つのHVからリソースが無くなるまで、VMを割り当てる
   HVのデータから、リソースが減ることを確認する

*/
func TestAssignHvforVm(t *testing.T) {

	/*
		type vmReq struct {
			name string
			cpu  int
			ram  int
		}

		tests := []struct {
			name string
			req  vmReq
			want string
			cpu  int
			ram  int
		}{
			{name: "1st", req: vmReq{name: "node1", cpu: 4, ram: 8}, want: "hv1", cpu: 6, ram: 56},
			{name: "2nd", req: vmReq{name: "node2", cpu: 4, ram: 8}, want: "hv1", cpu: 2, ram: 48},
			{name: "3rd", req: vmReq{name: "node3", cpu: 4, ram: 8}, want: "hv2", cpu: 6, ram: 56},
			{name: "4th", req: vmReq{name: "node4", cpu: 4, ram: 8}, want: "hv2", cpu: 2, ram: 48},
			{name: "5th", req: vmReq{name: "node5", cpu: 4, ram: 8}, want: "", cpu: 0, ram: 0},
			{name: "6th", req: vmReq{name: "node6", cpu: 2, ram: 8}, want: "hv1", cpu: 0, ram: 40},
			{name: "7th", req: vmReq{name: "node7", cpu: 1, ram: 8}, want: "hv2", cpu: 1, ram: 40},
			{name: "8th", req: vmReq{name: "node8", cpu: 1, ram: 8}, want: "hv2", cpu: 0, ram: 32},
		}
	*/
	/*
		var no = 1
		for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					vm := testVmCreate(tt.req.name, tt.req.cpu, tt.req.ram)
					hvName, key, txid, err := AssignHvforVm(Conn, vm)
					if err != nil {
						t.Errorf("assignHvforVm()")
					}
					fmt.Println("hv = ", hvName)
					fmt.Println("key = ", key)
					fmt.Println("txid = ", txid)

					// アサイン先
					if hvName != tt.want {
						t.Errorf("assignHvforVm() hvName %v", hvName)
					} else {
						if hvName != "" {
							// CPU残リソース
							hvs, err := GetHvByKey(Conn, hvName)
							if err != nil {
								t.Errorf("getHvByKey(%v) errer %v", hvName, err)
							}
							if hvs.FreeCpu != tt.cpu {
								t.Errorf("getHvByKey(%v) cpu=%v  Free CPU=%v", hvName, hvs.FreeCpu, tt.cpu)
							}
							// MEM残リソース
							if hvs.FreeMemory != tt.ram {
								t.Errorf("getHvByKey(%v) Memory=%v  Free RAM=%v", hvName, hvs.FreeMemory, tt.ram)
							}
						}
					}
				})
			no = no + 1
		}
	*/
}

// 仮想マシンの削除により、リソースの開放
func TestRemoveVmFromHV(t *testing.T) {

	/*
		type vmReq struct {
			name string
			cpu  int
			ram  int
		}

		tests := []struct {
			name string
			req  vmReq
			want string
			cpu  int
			ram  int
		}{
			{name: "1st", req: vmReq{name: "node1", cpu: 4, ram: 8}, want: "hv1", cpu: 4, ram: 48},
			{name: "2nd", req: vmReq{name: "node2", cpu: 4, ram: 8}, want: "hv1", cpu: 8, ram: 56},
			{name: "3rd", req: vmReq{name: "node3", cpu: 4, ram: 8}, want: "hv2", cpu: 4, ram: 40},
			{name: "4th", req: vmReq{name: "node4", cpu: 4, ram: 8}, want: "hv2", cpu: 8, ram: 48},
			{name: "5th", req: vmReq{name: "node5", cpu: 4, ram: 8}, want: "", cpu: 0, ram: 0},
			{name: "6th", req: vmReq{name: "node6", cpu: 2, ram: 8}, want: "hv1", cpu: 10, ram: 64},
			{name: "7th", req: vmReq{name: "node7", cpu: 1, ram: 8}, want: "hv2", cpu: 9, ram: 56},
			{name: "8th", req: vmReq{name: "node8", cpu: 1, ram: 8}, want: "hv2", cpu: 10, ram: 64},
		}

		var no = 1
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				vmKey, err := FindByHostname(Conn, tt.req.name)
				if err != nil && len(tt.req.name) > 0 {
					t.Errorf("findByVmKey(%v), want %v", tt.req.name, vmKey)
				}
				if len(vmKey) > 0 {
					err = RemoveVmFromHV(Conn, vmKey)
					if err != nil {
						t.Errorf("removeVmFromHV() err %v", err)
					} else {
						hvs, err := GetHvByKey(Conn, tt.want)
						if err != nil {
							t.Errorf("getHvByKey(%v) errer %v", tt.want, err)
						}
						if hvs.FreeCpu != tt.cpu {
							t.Errorf("getHvByKey(%v) cpu=%v  Free CPU=%v", tt.want, hvs.FreeCpu, tt.cpu)
						}
						// MEM残リソース
						if hvs.FreeMemory != tt.ram {
							t.Errorf("getHvByKey(%v) Memory=%v  Free RAM=%v", tt.want, hvs.FreeMemory, tt.ram)
						}
					}
				}
				no = no + 1
			})
		}
	*/
}

// キー毎のシーケンス番号
func TestSeq(t *testing.T) {
	err := CreateSeq(Conn, "test", 1, 1)
	if err != nil {
		t.Errorf("CreateSeq(%v,%v,%v,%v), err %v)", Conn, "test", 1, 1, err)
	}

	seq, err := GetSeq(Conn, "test")
	if seq != 1 {
		t.Errorf("GetSeq(%v, %v)  seq=%v, err=%v", Conn, "test", seq, err)
	}

	seq, err = GetSeq(Conn, "test")
	if seq != 2 {
		t.Errorf("GetSeq(%v, %v)  seq=%v, err=%v", Conn, "test", seq, err)
	}

	seq, err = GetSeq(Conn, "test")
	if seq != 3 {
		t.Errorf("GetSeq(%v, %v)  seq=%v, err=%v", Conn, "test", seq, err)
	}

	err = DelSeq(Conn, "test")
	if err != nil {
		t.Errorf("DelSeq(%v,%v)  err %v)", Conn, "test", err)
	}

}

// テストのクリーンナップ
func TestCleanup(t *testing.T) {
	var err error
	err = DelByKey(Conn, "vm")
	if err != nil {
		t.Errorf("delByKey()")
	}
	err = DelByKey(Conn, "hv")
	if err != nil {
		t.Errorf("delByKey()")
	}
	err = DelByKey(Conn, "serial")
	if err != nil {
		t.Errorf("delByKey()")
	}
	err = DelByKey(Conn, "SEQNO")
	if err != nil {
		t.Errorf("delByKey()")
	}

	Conn.Close()
}
