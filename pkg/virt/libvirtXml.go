package virt

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	//"github.com/google/uuid"
)

type Memory struct {
	Unit  string `xml:"unit,attr"`
	Value int    `xml:",chardata"`
}

type Vcpu struct {
	Unit  string `xml:"placement,attr"`
	Value int    `xml:",chardata"`
}

type Type struct {
	Arch    string `xml:"arch,attr"`
	Machine string `xml:"machine,attr"`
	Value   string `xml:",chardata"`
}

type Boot struct {
	Dev string `xml:"dev,attr"`
}

type OS struct {
	Type Type `xml:"type"`
	Boot Boot `xml:"boot"`
}

type Vmport struct {
	State string `xml:"state,attr"`
	Value string `xml:",chardata"`
}

type Features struct {
	Acpi   string `xml:"acpi"`
	Apic   string `xml:"apic"`
	Vmport Vmport `xml:"vmport"`
}

type CPU struct {
	Mode  string `xml:"mode,attr"`
	Check string `xml:"check,attr"`
}

type Timer struct {
	Name       string `xml:"name,attr"`
	Tickpolicy string `xml:"tickpolicy,attr,omitempty"`
	Present    string `xml:"present,attr,omitempty"`
}

type Clock struct {
	Timer  []Timer `xml:"timer"`
	Offset string  `xml:"offset,attr"`
}

type SuspendToMem struct {
	Enabled string `xml:"enabled,attr"`
}

type SuspendToDisk struct {
	Enabled string `xml:"enabled,attr"`
}

// パワーマネジメント
type PM struct {
	SuspendToMem  SuspendToMem  `xml:"suspend-to-mem"`
	SuspendToDisk SuspendToDisk `xml:"suspend-to-disk"`
}

type Emulator struct {
	Emulator string `xml:"string"`
}

type Driver struct {
	Name  string `xml:"name,attr"`
	Type  string `xml:"type,attr"`
	Cache string `xml:"cache,attr"`
	Io    string `xml:"io,attr"`
}

type Source struct {
	Dev string `xml:"dev,attr"`
}

type Target struct {
	Dev string `xml:"dev,attr"`
	Bus string `xml:"bus,attr"`
}

type Address struct {
	Type          string `xml:"type,attr,omitempty"`
	Domain        string `xml:"domain,attr,omitempty"`
	Bus           string `xml:"bus,attr,omitempty"`
	Slot          string `xml:"slot,attr,omitempty"`
	Function      string `xml:"function,attr,omitempty"`
	Multifunction string `xml:"multifunction,attr,omitempty"`
}

type Disk struct {
	Type    string  `xml:"type,attr"`
	Device  string  `xml:"device,attr"`
	Driver  Driver  `xml:"driver"`
	Source  Source  `xml:"source"`
	Target  Target  `xml:"target"`
	Address Address `xml:"address"`
}

type Master struct {
	Startport string `xml:"startport,attr,omitempty"`
}

type CTarget struct {
	Chassis string `xml:"chassis,attr,omitempty"`
	Port    string `xml:"port,attr,omitempty"`
}

type Controller struct {
	Type    string  `xml:"type,attr"`
	Index   string  `xml:"index,attr"`
	Model   string  `xml:"model,attr,omitempty"`
	Address Address `xml:"address,omitempty"` // 空の時には削除したい
	Master  Master  `xml:"master,omitempty"`  // 空の時には削除したい
	Target  CTarget `xml:"target,omitempty"`  // 空の時には削除したい
}

type Mac struct {
	Address string `xml:"address,attr"`
}

type ISource struct {
	Network   string `xml:"network,attr"`
	Portgroup string `xml:"portgroup,attr,omitempty"`
}

type Model struct {
	Type string `xml:"type,attr"`
}

type IAddress struct {
	Type     string `xml:"type,attr"`
	Domain   string `xml:"domain,attr,omitempty"`
	Bus      string `xml:"bus,attr,omitempty"`
	Slot     string `xml:"slot,attr,omitempty"`
	Function string `xml:"function,attr,omitempty"`
}

type Interface struct {
	Type    string   `xml:"type,attr"`
	Mac     Mac      `xml:"mac"`
	Source  ISource  `xml:"source"`
	Model   Model    `xml:"model"`
	Address IAddress `xml:"address"`
}

type SModel struct {
	Name string `xml:"name,attr"`
}

type STarget struct {
	Type  string `xml:"type,attr"`
	Port  string `xml:"port,attr"`
	Model SModel `xml:"model"`
}

type Serial0 struct {
	Type   string  `xml:"type,attr"`
	Target STarget `xml:"target"`
}

type ConTarget struct {
	Type string `xml:"type,attr"`
	Port string `xml:"port,attr"`
}

type Console struct {
	Type   string    `xml:"type,attr"`
	Target ConTarget `xml:"target"`
}

type ChanTarget struct {
	Type string `xml:"type,attr"`
	Name string `xml:"name,attr"`
}

type ChanAddress struct {
	Type       string `xml:"type,attr"`
	Controller string `xml:"controller,attr"`
	Bus        string `xml:"bus,attr"`
	Port       string `xml:"port,attr"`
}

type Channel struct {
	Type    string      `xml:"type,attr"`
	Target  ChanTarget  `xml:"target"`
	Address ChanAddress `xml:"address"`
}

type Inp_Address struct {
	Type string `xml:"type,attr,omitempty"`
	Bus  string `xml:"bus,attr,omitempty"`
	Port string `xml:"port,attr,omitempty"`
}

type Input struct {
	Type    string      `xml:"type,attr,omitempty"`
	Bus     string      `xml:"bus,attr,omitempty"`
	Address Inp_Address `xml:"address,omitempty"`
}

type Listen struct {
	Type string `xml:"type,attr"`
}

type Image struct {
	Compression string `xml:"compression,attr"`
}

type Graphics struct {
	Type     string `xml:"type,attr"`
	Autoport string `xml:"autoport,attr"`
	Listen   Listen `xml:"listen"`
	Image    Image  `xml:"image"`
}

type SndAddress struct {
	Type     string `xml:"type,attr"`
	Domain   string `xml:"domain,attr"`
	Bus      string `xml:"bus,attr"`
	Slot     string `xml:"slot,attr"`
	Function string `xml:"function,attr"`
}

type Sound struct {
	Model   string     `xml:"model,attr"`
	Address SndAddress `xml:"address"`
}

type VideoModel struct {
	Type    string `xml:"type,attr"`
	Ram     string `xml:"ram,attr"`
	Vram    string `xml:"vram,attr"`
	Vgamem  string `xml:"vgamem,attr"`
	Heads   string `xml:"heads,attr"`
	Primary string `xml:"primary,attr"`
}

type VideoAddress struct {
	Type     string `xml:"type,attr"`
	Domain   string `xml:"domain,attr"`
	Bus      string `xml:"bus,attr"`
	Slot     string `xml:"slot,attr"`
	Function string `xml:"function,attr"`
}

type Video struct {
	Model   VideoModel   `xml:"model"`
	Address VideoAddress `xml:"address"`
}

type RedirAddress struct {
	Type string `xml:"type,attr"`
	Bus  string `xml:"bus,attr"`
	Port string `xml:"port,attr"`
}

type Redirdev struct {
	Bus     string       `xml:"bus,attr"`
	Type    string       `xml:"type,attr"`
	Address RedirAddress `xml:"address"`
}

type MemAddress struct {
	Type     string `xml:"type,attr"`
	Domain   string `xml:"domain,attr"`
	Bus      string `xml:"bus,attr"`
	Slot     string `xml:"slot,attr"`
	Function string `xml:"function,attr"`
}

type Memballoon struct {
	Model   string     `xml:"model,attr"`
	Address MemAddress `xml:"address"`
}

type Backend struct {
	Model string `xml:"model,attr"`
	Value string `xml:",chardata"`
}

type RngAddress struct {
	Type     string `xml:"type,attr"`
	Domain   string `xml:"domain,attr"`
	Bus      string `xml:"bus,attr"`
	Slot     string `xml:"slot,attr"`
	Function string `xml:"function,attr"`
}

type Rng struct {
	Model   string     `xml:"model,attr"`
	Backend Backend    `xml:"backend"`
	Address RngAddress `xml:"address"`
}

// 　デバイス
type Devices struct {
	Emulator   string       `xml:"emulator"`
	Disk       []Disk       `xml:"disk"`
	Controller []Controller `xml:"controller"`
	Interface  []Interface  `xml:"interface"`
	Serial     []Serial0    `xml:"serial"`
	Console    []Console    `xml:"console"`
	Channel    []Channel    `xml:"channel"`
	Input      []Input      `xml:"input"`
	Graphics   []Graphics   `xml:"graphics"`
	Sound      []Sound      `xml:"sound"`
	Video      []Video      `xml:"video"`
	Redirdev   []Redirdev   `xml:"redirdev"`
	Memballoon []Memballoon `xml:"memballoon"`
	Rng        []Rng        `xml:"rng"`
}

// 　ドメイン(仮想マシン）の定義
type Domain struct {
	XMLName xml.Name `xml:"domain"`
	Type    string   `xml:"type,attr"`
	Name    string   `xml:"name"`
	//Uuid          uuid.UUID   `xml:"uuid"`
	Uuid          string   `xml:"uuid"`
	Metadata      string   `xml:"metadata"`
	Memory        Memory   `xml:"memory"`
	CurrentMemory Memory   `xml:"currentMemory"`
	Vcpu          Vcpu     `xml:"vcpu"`
	OS            OS       `xml:"os"`
	Features      Features `xml:"features"`
	Cpu           CPU      `xml:"cpu"`
	Clock         Clock    `xml:"clock"`
	On_poweroff   string   `xml:"on_poweroff"`
	On_reboot     string   `xml:"on_reboot"`
	On_crash      string   `xml:"on_crash"`
	PM            PM       `xml:"pm"`
	Devices       Devices  `xml:"devices"`
}

func ReadXml(fn string, xf interface{}) error {
	file, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := xml.NewDecoder(file)
	err = decoder.Decode(xf)
	if err != nil {
		return err
	}

	return nil
}

func WriteXml(fn string, xf interface{}) error {
	file, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := xml.NewEncoder(file)
	encoder.Indent("", "    ")
	err = encoder.Encode(xf)
	if err != nil {
		return err
	}

	return nil
}

func CreateVirtXML(domain Domain) string {
	// 画面へXMLを出力
	out, err := xml.MarshalIndent(domain, " ", "  ")
	if err != nil {
		fmt.Println("error: ", err)
	}

	// 読み取り処理のための変換
	reader := strings.NewReader(string(out))

	// NewScannerは bytes から読み取る新しい fileScanner を返す。
	fileScanner := bufio.NewScanner(reader)

	// スキャナの分割機能を設定
	fileScanner.Split(bufio.ScanLines)

	var xmlText string

	// 1行づつ処理する
	for fileScanner.Scan() {
		line := fileScanner.Text()

		// 削除するべき行の検知
		if strings.Contains(line, "<address></address>") {
			continue
		}
		if strings.Contains(line, "<master></master>") {
			continue
		}
		if strings.Contains(line, "<target></target>") {
			continue
		}
		xmlText = xmlText + line + "\n"
	}

	return xmlText
}

func SetVmParam(domain *Domain) {
	/***  構造体の値についての変更 ***/
	fmt.Println("VM Name: ", domain.Name)
	domain.Name = "VMMMMMMMMMMMMMMMMMMMMMMMMMMM"
	//fmt.Println("UUID: ", domain.Uuid)
	//domain.Uuid = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX";

	for _, s := range domain.Devices.Disk {
		if s.Type == "block" && s.Device == "disk" {
			fmt.Println(s.Source.Dev)
			fmt.Println(s.Target.Dev)
		}
	}
}
