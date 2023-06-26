package util

import (
	"testing"
	"fmt"
)

//
func TestReadConfig(t *testing.T) {

	var mountpoint string = "./root_lv"
	var hostname string = "server1"

	err := LinuxSetup_hostname(mountpoint, hostname)
	if err != nil {
		t.Errorf("LinuxSetup_hostname(%v,%v), err: %v", mountpoint, hostname, err)
	}


}

// ハイパーバイザーのREST-APIによる稼働チェック
func TestHypervisor(t *testing.T) {

	var dburl string = "http://hv1:2379"
	var node string = "hv1"

	x, err := CheckHypervisors(dburl, node)
	if err != nil {
		t.Errorf("CheckHypervisors(), err: %v", err)
	}
	fmt.Println("x = ", x[0].Nodename)


}

// ハイパーバイザーのREST-APIによる稼働チェック
func TestCheckHvVG(t *testing.T) {

	var dburl string = "http://hv1:2379"
	var node  string = "hv1"
	var vg    string = "vg1"

	err := CheckHvVG(dburl, node, vg)
	if err != nil {
		t.Errorf("CheckHvVG(), err: %v", err)
	}

}

