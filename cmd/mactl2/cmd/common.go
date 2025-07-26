package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"

	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
)

// 共通関数 GET
func ReqGet(apipath string, api string) (*http.Response, []byte, error) {
	res, err := http.Get(fmt.Sprintf("%s/%s", api, apipath))
	if err != nil {
		slog.Error("request by HTTP GET", "err", err)
		return nil, nil, err
	}
	defer res.Body.Close()

	byteBody, err := io.ReadAll(res.Body)
	if err != nil {
		slog.Error("reading body with HTTP-GET", "err", err)
		return nil, nil, err
	}
	return res, byteBody, err
}

// 仮想マシンのリスト表示
func ListVm(cnf cf.MarmotConfig, api string) error {
	_, body, err := ReqGet("virtualMachines", api)
	StateDsp := []string{"RGIST", "PROVI", "RUN", "STOP", "DELT", "Error"}
	if err != nil {
		slog.Error("list vms", "err", err)
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
			slog.Error("list vms in the cluster", "err", err)
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
		if match {
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

// 共通関数 POST
func ReqRest(cnf cf.MarmotConfig, apipath string, api string) (*http.Response, []byte, error) {
	byteJSON, _ := json.MarshalIndent(cnf, "", "    ")
	reqURL := fmt.Sprintf("%s/%s", api, apipath)
	request, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(byteJSON))
	if err != nil {
		slog.Error("request by HTTP-POST", "err", err)
		return nil, nil, err
	}

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		slog.Error("http client", "err", err)
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
	return resp, body, err
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
