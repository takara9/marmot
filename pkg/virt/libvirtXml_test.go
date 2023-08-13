package virt

import (
	"fmt"
	"testing"
	"time"
)

var in string = "test-data/vm-1.xml"
var out string = "test-data/out-1.xml"
var domain Domain

func TestReadXml(t *testing.T) {

	err := ReadXml(in, &domain)
	if err != nil {
		t.Errorf("ReadXml(%v), err: %v", in, err)
	}
}

func TestSetVmParam(t *testing.T) {
	SetVmParam(&domain)
}

func TestCreateVirtXML(t *testing.T) {
	xmlText := CreateVirtXML(domain)
	fmt.Println(xmlText)
}

func TestWriteXml(t *testing.T) {

	err := WriteXml(out, &domain)
	if err != nil {
		t.Errorf("WriteXml(%v), err: %v", out, err)
	}
}

func TestListAllVm(t *testing.T) {
	url := "qemu:///system"
	list, err := ListAllVm(url)
	if err != nil {
		t.Errorf("ListAllVm(%v), err: %v", url, err)
	}

	for _, name := range list {
		fmt.Println("VM Name = ", name)
	}
}

func TestCreateVm(t *testing.T) {
	url := "qemu:///system"
	fn := "test-data/vm-test.xml"

	err := CreateStartVM(url, fn)
	if err != nil {
		t.Errorf("CreateVM(%v,%v), err: %v", url, fn, err)
	}
	time.Sleep(time.Second * 30)
}

func TestAttachDevStorage(t *testing.T) {
	url := "qemu:///system"
	vm := "vm-test"
	fn := "test-data/storage-lv08.xml"

	err := AttachDev(url, fn, vm)
	if err != nil {
		t.Errorf("AttachDev(%v,%v,%v), err: %v", url, fn, vm, err)
	}
}

func TestAttachDevNIC(t *testing.T) {
	url := "qemu:///system"
	vm := "vm-test"
	fn := "test-data/nic-vlan1001.xml"

	err := AttachDev(url, fn, vm)
	if err != nil {
		t.Errorf("AttachDev(%v,%v,%v), err: %v", url, fn, vm, err)
	}
}

func TestDestroyVM(t *testing.T) {
	url := "qemu:///system"
	vm := "vm-test"

	err := DestroyVM(url, vm)
	if err != nil {
		t.Errorf("DestroyVM(%v,%v), err: %v", url, vm, err)
	}
}
