package lvm

import (
	"testing"
	"fmt"
	//"time"
)

// テストボリューム
var vg = "vg1"
var lv = "lv20"
var sz uint64 = 1024 * 1024 * 1024 * 4  // GB

var ssz uint64 = 1024 * 1024 * 1024 * 1  // GB
var lvt = "lv01"
var slv = "lv21"

/*
func TestCreateLV(t *testing.T) {

	err := CreateLV(vg, lv, sz)
	if err != nil {
		t.Errorf("CreateLV(%v,%v,%v), err: %v", vg,lv,sz,err)
	}
	time.Sleep(time.Second * 30)
	err = IsExist(vg, lv)
	if err != nil {
		t.Errorf("IsExist(%v,%v), err: %v", vg,lv,err)
	}

}

func TestRemoveLV(t *testing.T) {
	err := RemoveLV(vg, lv)
	if err != nil {
		t.Errorf("RemoveLV(%v,%v), err: %v", vg,lv,err)
	}
}

func TestCreateSnapshot(t *testing.T) {
	err := CreateSnapshot(vg, lvt, slv, ssz)
	if err != nil {
		t.Errorf("CreateSnapshot(%v,%v,%v,%v), err: %v", vg, lvt, slv, ssz, err)
	}
	time.Sleep(time.Second * 30)
	err = RemoveLV(vg, slv)
	if err != nil {
		t.Errorf("RemoveLV(%v,%v), err: %v", vg,slv,err)
	}
}
*/

func TestCheckVG(t *testing.T) {
	size,free, err := CheckVG(vg)
	if err != nil {
		t.Errorf("CheckVG(%v) %v,%v, err: %v", vg, size,free,err)
	}
	sizeg := size/1024/1024/1024
	freeg := free/1024/1024/1024
	fmt.Println("size = ",sizeg)
	fmt.Println("free = ",freeg)
}
