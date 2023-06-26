package config

import (
	"testing"
	"fmt"
)

const input1   string = "testdata/ceph-cluster.yaml"
const output1  string = "testdata/ceph-cluster-out.yaml"
var mc MarmotConfig

// コンフィグファイルの読み取り
func TestReadConfig(t *testing.T) {
	err := ReadConfig(input1, &mc)
	if err != nil {
		t.Errorf("readConfig(%v), err: %v", input1, err)
	}
	fmt.Println("mc = ",mc)
}

// コンフィグファイルの書き込み
func TestWriteConfig(t *testing.T) {
	err := WriteConfig(output1, mc)
	if err != nil {
		t.Errorf("writeConfig(%v), err %v", output1, err)
	}
	fmt.Println("mc = ",mc)
}
