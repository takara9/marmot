/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/types"
)

// exportCmd represents the export command
var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "marmot のデータをJSON形式でエクスポートする",
	Long:  `marmot の管理データをJSON形式でエクスポートします。バージョンアップ前に本コマンドでデータをバックアップして、バージョンを更新した後にインポートすることでデータの移行を実施できます。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return exportConfig()
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
	exportCmd.PersistentFlags().StringVarP(&etcdUrl, "etcdurl", "e", "http://localhost:2379", "URL address of the etcd server")
}

func writeJsonFile(filename string, data interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		slog.Error("Failed to convert Json from stuct", "error", err)
		return err
	}

	f, err := os.Create(filename)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to open %v", filename), "error", err)
		return err
	}
	defer f.Close()

	_, err = f.Write(jsonBytes)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to write %v", filename), "error", err)
		return err
	}
	return nil
}

func exportConfig() error {


	// ディレクトリを作成
	fileInfo, err := os.Lstat("./")
	if err != nil {
		slog.Error("current dir check", "err", err)
		return err
	}

	fileMode := fileInfo.Mode()
	unixPerms := fileMode & os.ModePerm
	if err := os.Mkdir("marmot-config/", unixPerms); err != nil {
		slog.Error("making directory", "err", err)
		return err
	}

	// データベースと接続
	d, err := db.NewDatabase(etcdUrl)
	if err != nil {
		slog.Error("Failed to connect to etcd", "error", err)
		return err
	}

	// バージョンを書き込むこと


	// ハイパーバイザー
	var hvs []types.Hypervisor
	err = d.GetHypervisors(&hvs)
	if err != nil {
		slog.Error("Failed to get hypervisor data", "error", err)
		return err
	}
	if err := writeJsonFile("marmot-config/marmot-hv.json", hvs); err != nil {
		slog.Error("Failed to write hypervisor data", "error", err)
		return err
	}

	// OSイメージテンプレート
	var osit []types.OsImageTemplate
	err = d.GetOsImgTempes(&osit)
	if err != nil {
		slog.Error("Failed to get os image templates data", "error", err)
		return err
	}
	if err := writeJsonFile("marmot-config/marmot-os-temp.json", osit); err != nil {
		slog.Error("Failed to write OS image templates data", "error", err)
		return err
	}

	// シーケンス番号
	var seqs []types.VmSerial
	err = d.GetSeqs(&seqs)
	if err != nil {
		slog.Error("Failed to get Serial numbers data", "error", err)
		return err
	}
	if err := writeJsonFile("marmot-config/marmot-seq-data.json", seqs); err != nil {
		slog.Error("Failed to write Serial numbers data", "error", err)
		return err
	}

	//仮想マシン
	var vms []types.VirtualMachine
	err = d.GetVmsStatus(&vms)
	if err != nil {
		slog.Error("Failed to get data of virtual machines", "error", err)
		return err
	}
	if err := writeJsonFile("marmot-config/marmot-vm-data.json", vms); err != nil {
		slog.Error("Failed to write data of virtual machines data", "error", err)
		return err
	}

	return nil
}
