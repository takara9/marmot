/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/types"
)

// importCmd represents the migrate command
var importCmd = &cobra.Command{
	Use:   "import",
	Short: "エクスポートされたデータをmarmotdのデータベースに読み込む",
	Long:  `エクスポートされた marmotd の管理データをインポートします。バージョンアップ前に本コマンドでデータをバックアップして、バージョンを更新した後にインポートすることでデータの移行が実施できます。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return importConfig()
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.PersistentFlags().StringVarP(&etcdUrl, "etcdurl", "e", "http://localhost:2379", "URL address of the etcd server")
	importCmd.PersistentFlags().StringVarP(&exportFilename, "filename", "f", "marmot-config-backup.zip", "Backup file name")
}

func unZip(zipFilename, distDir string) error {
	r, err := zip.OpenReader(zipFilename)
	if err != nil {
		slog.Error("impossible to open zip reader", zipFilename, err)
		return err
	}
	defer r.Close()

	err = os.MkdirAll(distDir, 0755)
	if err != nil {
		slog.Error("impossible to create destination directory", distDir, err)
		return err
	}

	// Zipに含まれる各ファイルを処理
	for _, f := range r.File {
		fmt.Printf("Unzipping %s:\n", f.Name)
		rc, err := f.Open()
		if err != nil {
			slog.Error("impossible to open zip reader", zipFilename, err)
			return err
		}
		defer rc.Close()

		newFilePath := filepath.Join(distDir, f.Name)
		// ディレクトリの場合
		if f.FileInfo().IsDir() {
			err = os.MkdirAll(newFilePath, 0777)
			if err != nil {
				slog.Error("impossible to MkdirAll", newFilePath, err)
				return err
			}
			continue
		}
		// ファイルの場合
		uncompressedFile, err := os.Create(newFilePath)
		if err != nil {
			slog.Error("impossible to create uncompressed", newFilePath, err)
			return err
		}
		// ファイルを書き込み
		_, err = io.Copy(uncompressedFile, rc)
		if err != nil {
			slog.Error("impossible to copy file", newFilePath, err)
			return err
		}
	}

	return nil
}

func importConfig() error {
	slog.Debug("インポートの実行")
	// 一時ディレクトリ名を生成する
	workDir, err := os.MkdirTemp("/tmp", "marmot-config-work")
	if err != nil {
		slog.Error("failed to creaate working directory", "err", err)
		return err
	}
	defer os.RemoveAll(workDir)

	// Unzip file
	if err := unZip(exportFilename, workDir); err != nil {
		slog.Error("failed to uncompress zip file on working directory", "err", err)
		return err
	}

	// データベースと接続
	d, err := db.NewDatabase(etcdUrl)
	if err != nil {
		slog.Error("Failed to connect to etcd", "error", err)
		return err
	}

	// ハイパーバイザー
	// read marmot-hv.json
	path := filepath.Join(workDir, "marmot-hv.json")
	jsonBytes, err := os.ReadFile(path)
	if err != nil {
		slog.Error("Failed to read json file in working dir", "error", err)
		return err
	}
	var buf []api.Hypervisor
	if err := json.Unmarshal(jsonBytes, &buf); err != nil {
		slog.Error("Failed to unmarshal", "error", err)
		return err
	}
	for _, v := range buf {
		d.PutJSON(*v.Key, v)

	}

	// バージョン
	// marmot-version.json
	path = filepath.Join(workDir, "marmot-version.json")
	jsonBytes, err = os.ReadFile(path)
	if err != nil {
		slog.Error("Failed to read json file in working dir", "error", err)
		return err
	}
	var bufVer api.Version
	if err := json.Unmarshal(jsonBytes, &bufVer); err != nil {
		slog.Error("Failed to unmarshal", "error", err)
		return err
	}
	slog.Info("Imported version information", "importedVersion", bufVer)
	// TODO: バージョンの整合性チェックと移行処理を実装する

	// データベースへバージョンの書き込み
	serverVersion := version
	ver := api.Version{
		ServerVersion: &serverVersion,
	}
	if err := d.SetVersion(ver); err != nil {
		slog.Error("Failed to set version", "error", err)
		return err
	}

	// OSイメージテンプレート
	// marmot-os-temp.json
	path = filepath.Join(workDir, "marmot-os-temp.json")
	jsonBytes, err = os.ReadFile(path)
	if err != nil {
		slog.Error("Failed to read json file in working dir", "error", err)
		return err
	}
	var buf2 []types.OsImageTemplate
	if err := json.Unmarshal(jsonBytes, &buf2); err != nil {
		slog.Error("Failed to unmarshal", "error", err)
		return err
	}
	// 配列データを1件ずつ登録している。正しいのか？

	for _, v := range buf2 {
		slog.Debug("marmot-os-temp.json", "key", v.Key)
		if err := d.PutJSON(v.Key, v); err != nil {
			slog.Error("Failed to put OS image template data", "error", err, "key", v.Key)
			return err
		}
	}

	// シーケンス番号
	// marmot-seq-data.json
	path = filepath.Join(workDir, "marmot-seq-data.json")
	jsonBytes, err = os.ReadFile(path)
	if err != nil {
		slog.Error("Failed to read json file in working dir", "error", err)
		return err
	}
	var buf3 []types.VmSerial
	if err := json.Unmarshal(jsonBytes, &buf3); err != nil {
		slog.Error("Failed to unmarshal", "error", err)
		return err
	}
	for _, v := range buf3 {
		d.PutJSON(v.Key, v)
	}

	//Read 仮想マシン
	// marmot-vm-data.json
	path = filepath.Join(workDir, "marmot-vm-data.json")
	jsonBytes, err = os.ReadFile(path)
	if err != nil {
		slog.Error("Failed to read json file in working dir", "error", err)
		return err
	}
	var buf4 []api.VirtualMachine
	if err := json.Unmarshal(jsonBytes, &buf4); err != nil {
		slog.Error("Failed to unmarshal", "error", err)
		return err
	}
	for _, v := range buf4 {
		d.PutJSON(*v.Key, v)
	}

	return nil
}
