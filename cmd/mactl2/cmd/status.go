/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	cf "github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/db"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status VMs",
	Long:  `Show status of virtual machines.`,
	Run: func(cmd *cobra.Command, args []string) {

		fmt.Println("status called")
		fmt.Println("conf=", DefaultConfig)
		ListVm(cnf, ApiUrl)

	},
}

func init() {
	rootCmd.AddCommand(statusCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// statusCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// statusCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

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
