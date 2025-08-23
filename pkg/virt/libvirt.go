package virt

import (
	"io"
	"log/slog"
	"os"
	"time"

	"libvirt.org/go/libvirt"
)

func ReadFileOnMem(fn string) ([]byte, error) {
	xmlFile, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer xmlFile.Close()

	byteValue, err := io.ReadAll(xmlFile)
	if err != nil {
		return nil, err
	}
	return byteValue, err
}

func ListAllVm(url string) ([]string, error) {
	var nameList []string

	conn, err := libvirt.NewConnect(url)
	if err != nil {
		return nameList, err
	}
	defer conn.Close()

	doms, err := conn.ListAllDomains(libvirt.ConnectListAllDomainsFlags(libvirt.CONNECT_LIST_DOMAINS_ACTIVE))
	if err != nil {
		return nameList, err
	}

	for _, dom := range doms {
		name, err := dom.GetName()
		if err != nil {
			return nameList, err
		}
		nameList = append(nameList, name)
		dom.Free()
	}
	return nameList, nil
}

func CreateStartVM(url string, filename string) error {

	byteValue, err := ReadFileOnMem(filename)
	if err != nil {
		return err
	}

	conn, err := libvirt.NewConnect(url)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Create VM
	dom, err := conn.DomainDefineXML(string(byteValue))
	if err != nil {
		return err
	}
	// Start VM
	err = dom.Create()
	if err != nil {
		return err
	}

	//time.Sleep(2000 * time.Millisecond)
	//　オートスタートを設定しないと、HVの再起動からの復帰時、停止している。
	err = dom.SetAutostart(true)
	if err != nil {
		return err
	}

	return nil
}

func DestroyVM(url string, vmname string) error {

	conn, err := libvirt.NewConnect(url)
	if err != nil {
		slog.Error("", "err", err)
	}
	defer conn.Close()

	dom, err := conn.LookupDomainByName(vmname)
	if err != nil {
		slog.Error("", "err", err)
		// ドメインが存在しない場合は、エラーリターン
		return err
	}

	_, _, err = dom.GetState()
	if err != nil {
		slog.Error("", "err", err)
	}
	err = dom.Destroy()
	if err != nil {
		slog.Error("", "err", err)
	}

	time.Sleep(time.Second * 10)

	dom.Undefine()
	if err != nil {
		slog.Error("", "err", err)
	}

	dom.Free()
	return nil
}

/*
デバイスの追加、VMが実行中でないと動作しない
*/
func AttachDev(url string, fn string, vmname string) error {

	conn, err := libvirt.NewConnect(url)
	if err != nil {
		return err
	}
	defer conn.Close()

	byteValue, err := ReadFileOnMem(fn)
	if err != nil {
		return err
	}

	dom, err := conn.LookupDomainByName(vmname)
	if err != nil {
		return err
	}

	err = dom.AttachDevice(string(byteValue))
	if err != nil {
		return err
	}
	dom.Free()

	return nil
}

// 仮想マシンの停止
func StopVM(url string, vmname string) error {

	conn, err := libvirt.NewConnect(url)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	defer conn.Close()

	dom, err := conn.LookupDomainByName(vmname)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	// 仮想マシンの停止
	_, _, err = dom.GetState()
	if err != nil {
		slog.Error("", "err", err)
	}

	err = dom.Shutdown()
	if err != nil {
		slog.Error("", "err", err)
	}

	time.Sleep(time.Second * 10)

	dom.Free()
	return nil
}

// 仮想マシンの停止
func StartVM(url string, vmname string) error {

	conn, err := libvirt.NewConnect(url)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}
	defer conn.Close()

	dom, err := conn.LookupDomainByName(vmname)
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	err = dom.Create()
	if err != nil {
		slog.Error("", "err", err)
		return err
	}

	time.Sleep(time.Second * 10)

	dom.Free()
	return nil
}
