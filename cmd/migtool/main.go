package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"marmot.io/db"
	"marmot.io/util"
)

func main() {
	// ホームディレクトリの.config_marmotから
	// APIサーバーとetcdサーバーのURLを取得
	_, cnf, err := util.ReadHvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// データベースに登録 etcdへ接続
	d, err := db.NewDatabase(cnf.EtcdServerUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// データの変換
	var old []db.HypervisorOld
	d.GetHypervisorsOld(&old)
	fmt.Println("Old Hypervisors:")
	printHypervisorsOld(old)

	// 変換実行
	fmt.Println("Converting...")
	new := convertOldToNew(old)

	fmt.Println("New Hypervisors:")
	printHypervisors(new)

	// 古いデータを削除
	if err = deleteOldData(old, d); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// 新しいデータを登録
	if err = putNewData(new, d); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Migration completed successfully.")
}

func putNewData(new []db.Hypervisor, d *db.Database) error {
	for _, newHv := range new {
		err := d.PutDataEtcd(newHv.Key, newHv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return err
		}
	}
	return nil
}

func deleteOldData(old []db.HypervisorOld, d *db.Database) error {
	for _, hv := range old {
		err := d.DelByKey(hv.Key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return err
		}
	}
	return nil
}

func convertOldToNew(old []db.HypervisorOld) []db.Hypervisor {
	var new []db.Hypervisor
	new = make([]db.Hypervisor, len(old))
	for i, hv := range old {
		new[i].Nodename = hv.Nodename
		new[i].Cpu = hv.Cpu
		new[i].Memory = hv.Memory
		new[i].IpAddr = hv.IpAddr
		new[i].FreeCpu = hv.FreeCpu
		new[i].FreeMemory = hv.FreeMemory
		new[i].Key = hv.Key
		new[i].Status = hv.Status
		new[i].StgPool = append(new[i].StgPool, hv.StgPool...)
		// ポード番号が無いので追加
		new[i].Port = 8750 // デフォルトポート番号
	}
	return new
}

func printHypervisors(hvs []db.Hypervisor) {
	for _, hv := range hvs {
		fmt.Printf("Nodename: %s, CPU: %d, Memory: %d, IP: %s, FreeCPU: %d, FreeMemory: %d, Port: %d, Key: %s, Status: %v\n",
			hv.Nodename, hv.Cpu, hv.Memory, hv.IpAddr, hv.FreeCpu, hv.FreeMemory, hv.Port, hv.Key, hv.Status)
		for _, sp := range hv.StgPool {
			fmt.Printf("  -        Type: %s\n", sp.Type)
			fmt.Printf("  -    VolGroup: %s\n", sp.VolGroup)
			fmt.Printf("  -       VgCap: %d GB\n", sp.VgCap)
			fmt.Printf("  -     FreeCap: %d GB\n", sp.FreeCap)
		}
	}
}

func printHypervisorsOld(hvs []db.HypervisorOld) {
	for _, hv := range hvs {
		fmt.Printf("Nodename: %s, CPU: %d, Memory: %d, IP: %s, FreeCPU: %d, FreeMemory: %d, Key: %s, Status: %v\n",
			hv.Nodename, hv.Cpu, hv.Memory, hv.IpAddr, hv.FreeCpu, hv.FreeMemory, hv.Key, hv.Status)
		for _, sp := range hv.StgPool {
			fmt.Printf("  -        Type: %s\n", sp.Type)
			fmt.Printf("  -    VolGroup: %s\n", sp.VolGroup)
			fmt.Printf("  -       VgCap: %d GB\n", sp.VgCap)
			fmt.Printf("  -     FreeCap: %d GB\n", sp.FreeCap)
		}
	}
}

func YesNoPrompt(label string, def bool) bool {
	choices := "Y/n"
	if !def {
		choices = "y/N"
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "%s (%s) ", label, choices) // Prompt the user
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" { // If input is empty, use the default
			return def
		}

		input = strings.ToLower(input)
		if input == "y" || input == "yes" {
			return true
		}
		if input == "n" || input == "no" {
			return false
		}
		// If input is invalid, loop and ask again
	}
}
