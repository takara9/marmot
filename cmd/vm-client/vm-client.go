package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	cf "github.com/takara9/marmot/pkg/config"
	db "github.com/takara9/marmot/pkg/db"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BODYのJSONエラーメッセージ処理用
type msg struct {
	Msg string
}

type DefaultConfig struct {
	ApiServerUrl string `yaml:"api_server"`
}

// メイン
func main() {

	// コンフィグファイルの読み取り
	var DefaultConfig DefaultConfig
	cf.ReadConfig(filepath.Join(os.Getenv("HOME"), ".config_marmot"), &DefaultConfig)

	// パラメータの取得
	ccf := flag.String("ccf", "cluster-config.yaml", "Config YAML file")
	api := flag.String("api", "", "VM Apiserver URL http://127.0.0.1:8750")
	auto := flag.Bool("auto", false, "apply ansible playvook")
	flag.Parse()
	arg := flag.Args()

	// パラメーター > コンフィグ
	ApiUrl := ""
	if len(DefaultConfig.ApiServerUrl) > 0 {
		ApiUrl += DefaultConfig.ApiServerUrl
	}
	if len(*api) > 0 {
		ApiUrl = *api
	}

	// コンフィグ無しで実行可能なサブコマンド
	if len(arg) > 0 {
		switch arg[0] {
		case "hvs":
			ListHv(ApiUrl)
			return
		case "global-status":
			fmt.Printf("\n               *** SYSTEM STATUS ***\n")
			ListHv(ApiUrl)
			fmt.Printf("\n")
			GlobalListVm(ApiUrl)
			fmt.Printf("\n")
			return
		}
	}

	// サブコマンド
	var cnf cf.MarmotConfig
	err := cf.ReadConfig(*ccf, &cnf)
	if err != nil {
		log.Fatal(err)
		return
	} else {
		switch arg[0] {
		case "gen-inv":
			generate_all(cnf)
		case "get-kubeconf":
			GetKubeconf(cnf) // 途上
		case "list":
			ListVm(cnf, ApiUrl)
		case "status":
			ListVm(cnf, ApiUrl)
		case "vm":
			DetailVm(cnf, ApiUrl, arg)
		case "create":
			ReqRest(cnf, "createCluster", ApiUrl)
			if *auto {
				apply_playbook(cnf)
			}
		case "destroy":
			ReqRest(cnf, "destroyCluster", ApiUrl)
		case "start":
			ReqRest(cnf, "startCluster", ApiUrl)
		case "stop":
			ReqRest(cnf, "stopCluster", ApiUrl)
		}
	}
}

// 共通関数 GET
func ReqGet(apipath string, api string) (*http.Response, []byte, error) {

	res, err := http.Get(fmt.Sprintf("%s/%s", api, apipath))
	if err != nil {
		log.Fatal(err)
		return nil, nil, err
	}
	defer res.Body.Close()

	byteBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
		return nil, nil, err
	}
	return res, byteBody, err
}

// 共通関数 POST
func ReqRest(cnf cf.MarmotConfig, apipath string, api string) (*http.Response, []byte, error) {
	byteJSON, _ := json.MarshalIndent(cnf, "", "    ")
	reqURL := fmt.Sprintf("%s/%s", api, apipath)
	request, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		log.Println(err)
		return nil, nil, err
	}

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		log.Println(err)
		return resp, nil, err
	}
	defer resp.Body.Close()

	// レスポンスを取得する
	body, err := io.ReadAll(resp.Body)
	var ErrMsg msg
	if resp.StatusCode != 200 {
		fmt.Println("失敗")
		json.Unmarshal(body, &ErrMsg)
		fmt.Println("エラーメッセージ:", ErrMsg.Msg)
	} else {
		fmt.Println("成功終了")
	}

	//fmt.Println("response Status:", response.Status)
	//fmt.Println("response Headers:", response.Header)
	//fmt.Println("response Body:", string(body))

	return resp, body, err
}

// 仮想マシンのリスト表示
func ListVm(cnf cf.MarmotConfig, api string) error {

	_, body, err := ReqGet("virtualMachines", api)
	StateDsp := []string{"RGIST", "PROVI", "RUN", "STOP", "DELT", "Error"}
	if err != nil {
		log.Println(err)
		return err
	}

	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.Token()
	fmt.Printf("%-10s %-16s %-6s %-5s %-20s %-4v  %-6v %-15v %-15v ",
		"CLUSTER", "VM-NAME", "H-Visr", "STAT", "VKEY", "VCPU", "RAM", "PubIP", "PriIP")
	fmt.Printf("%-20s", "DATA STORAGE")
	fmt.Printf("\n")
	for dec.More() {
		// クラスタ名と仮想マシンが一致したものだけリスト
		var vm db.VirtualMachine
		err := dec.Decode(&vm)
		if err != nil {
			log.Fatal(err)
		}
		// フィルター処理
		match := false
		if cnf.ClusterName == vm.ClusterName {
			for _, spec := range cnf.VMSpec {
				if spec.Name == vm.Name {
					match = true
					break
				}
			}
		}
		// 表示
		if match == true {
			fmt.Printf("%-10s %-16s %-6s %-5s %-20s %-4v  %-6v %-15v %-15v ",
				vm.ClusterName, vm.Name, vm.HvNode, StateDsp[vm.Status],
				vm.Key, vm.Cpu, vm.Memory, vm.PrivateIp, vm.PublicIp)
			for _, dv := range vm.Storage {
				fmt.Printf("%-4d", dv.Size)
			}
			fmt.Printf("\n")
		}
	}
	dec.Token()
	return nil
}

// 全体の仮想マシンのリスト表示
func GlobalListVm(api string) error {
	_, body, err := ReqGet("virtualMachines", api)
	StateDsp := []string{"RGIST", "PROVI", "RUN", "STOP", "DELT", "Error"}
	if err != nil {
		log.Println(err)
		return err
	}

	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.Token()
	fmt.Printf("%-10s %-16s %-6s %-5s %-20s %-4v  %-6v %-15v %-15v ",
		"CLUSTER", "VM-NAME", "H-Visr", "STAT", "VKEY", "VCPU", "RAM", "PubIP", "PriIP")
	fmt.Printf("%-20s", "DATA STORAGE")
	fmt.Printf("\n")
	for dec.More() {
		var vm db.VirtualMachine
		err := dec.Decode(&vm)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-10s %-16s %-6s %-5s %-20s %-4v  %-6v %-15v %-15v ",
			vm.ClusterName, vm.Name, vm.HvNode, StateDsp[vm.Status],
			vm.Key, vm.Cpu, vm.Memory, vm.PrivateIp, vm.PublicIp)
		for _, dv := range vm.Storage {
			fmt.Printf("%-4d", dv.Size)
		}
		fmt.Printf("\n")
	}
	dec.Token()
	return nil
}

// ハイパーバイザーのリスト表示
func ListHv(api string) error {
	_, body, err := ReqGet("hypervisors", api)
	if err != nil {
		log.Println(err)
		return err
	}
	status := [3]string{"HLT", "ERR", "RUN"}

	// Bodyの処理
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.Token()
	fmt.Printf("%-10s %-3v %-15v %-8v  %-12v   %-12v", "HV-NAME", "ONL", "IPaddr", "VCPU", "RAM(MB)", "Storage(GB)")
	fmt.Printf("\n")
	for dec.More() {
		var hv db.Hypervisor
		err := dec.Decode(&hv)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-10s %-3v %-15v %4d/%-4d %6d/%-6d  ",
			hv.Nodename, status[hv.Status], hv.IpAddr, hv.FreeCpu, hv.Cpu, hv.FreeMemory, hv.Memory)
		for _, v := range hv.StgPool {
			fmt.Printf("%v(%v): %5d/%-5d ", v.VolGroup, v.Type, v.FreeCap, v.VgCap)
		}
		fmt.Printf("\n")
	}
	dec.Token()
	return nil
}

// Ansible Playbook の適用
func apply_playbook(cnf cf.MarmotConfig) {

	for {
		out, err := exec.Command("ansible", "-i", "hosts_kvm", "-m", "ping", "all").Output()
		if err != nil {
			fmt.Println("待機中...")
			time.Sleep(500 * time.Millisecond)
		} else {
			fmt.Println(string(out))
			break
		}
	}

	for _, spec := range cnf.VMSpec {
		path := fmt.Sprintf("playbook/%v", spec.AnsiblePB)
		out, err := exec.Command("ansible-playbook", "-i", "hosts_kvm", path).Output()
		if err != nil {
			fmt.Println("err = ", err)
		} else {
			fmt.Println(string(out))
		}
	}
}

// 仮想マシンの詳細表示
func DetailVm(cnf cf.MarmotConfig, api string, arg []string) error {

	_, body, err := ReqGet("virtualMachines", api)
	if err != nil {
		log.Println(err)
		return err
	}
	StateDsp := []string{"RGIST", "PROVI", "RUN", "STOP", "DELT", "Error"}

	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.Token()
	match := false
	var vm db.VirtualMachine

	for dec.More() {
		// クラスタ名と仮想マシンが一致したものだけリスト
		err := dec.Decode(&vm)
		if err != nil {
			log.Fatal(err)
		}
		// フィルター処理
		if arg[1] == vm.ClusterName {
			if arg[2] == vm.Name {
				match = true
				break
			}
		}
	}
	// 表示
	if match == true {
		fmt.Printf("\n*** Virtual Machine Detail Info ***\n")
		fmt.Printf("\n")
		fmt.Printf("Cluster Name : %s \n", vm.ClusterName)
		fmt.Printf("Virtual Machine Name : %s \n", vm.Name)
		fmt.Printf("UUID : %s\n", vm.Uuid)
		fmt.Printf("Hypervisor : %s\n", vm.HvNode)
		fmt.Printf("Key: %s \n", vm.Key)
		fmt.Printf("Create Time: %s\n", vm.Ctime)
		fmt.Printf("Start  Time: %s\n", vm.Stime)
		fmt.Printf("Status : %s \n", StateDsp[vm.Status])
		fmt.Printf("CPU : %d \n", vm.Cpu)
		fmt.Printf("Memory(MB) : %d\n", vm.Memory)
		fmt.Printf("Private IP addr : %s\n", vm.PrivateIp)
		fmt.Printf("Public  IP addr : %s\n", vm.PublicIp)
		fmt.Printf("\n")
		fmt.Printf("OS Storage\n")
		fmt.Printf("    Volume Group Name : %s\n", vm.OsVg)
		fmt.Printf("    Logical Volume Name : %s\n", vm.OsLv)
		fmt.Printf("    OS Variant : %s\n", vm.OsVariant)
		fmt.Printf("\n")
		fmt.Printf("Data Storage\n")
		for _, v := range vm.Storage {
			fmt.Printf("    Storage Name : %s\n", v.Name)
			fmt.Printf("    Size(GB) : %d\n", v.Size)
			fmt.Printf("    Volume Group Name : %s\n", v.Vg)
			fmt.Printf("    Logical Volume Name : %s\n", v.Lv)
			fmt.Printf("    Path : %s\n", v.Path)
		}
		fmt.Printf("\n")
		fmt.Printf("Comment: %s \n", vm.Comment)
		fmt.Printf("Ansible Playbook: %s \n", vm.Playbook)
		fmt.Printf("\n")
	}
	dec.Token()
	return nil
}
