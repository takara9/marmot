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
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/types"
)

var exportFilename string

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
	exportCmd.PersistentFlags().StringVarP(&exportFilename, "filename", "f", "marmot-config-backup.zip", "Backup file name")
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

func createZip(zipFilename string, srcDirectory string) error {
	zipFile, err := os.Create(zipFilename)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to create zip file %v", zipFilename), "err", err)
		return err
	}
	defer zipFile.Close()

	// ZIPライターの初期化
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 圧縮したいファイルのリスト
	files, err := os.ReadDir(srcDirectory)
	if err != nil {
		slog.Error(fmt.Sprintf("%v can not read directory", srcDirectory), "err", err)
		return err
	}

	// ディレクトリ内のファイルを読む
	var filesToZip []string
	for _, file := range files {
		slog.Debug(fmt.Sprintf("file: %v in %v", file.Name(), srcDirectory))
		if file.IsDir() {
			slog.Error(fmt.Sprintf("Not allow recursive directory", file.Name()), "err", err)
			return err
		}
		filesToZip = append(filesToZip, filepath.Join(srcDirectory, file.Name()))
	}

	// パスを生成して、ZIPファイルに追加する
	for _, targetFilePath := range filesToZip {
		if err := addFileToZip(targetFilePath, zipWriter); err != nil {
			slog.Error(fmt.Sprintf("Can`t add %v to zip file", targetFilePath), "err", err)
			return err
		}
	}
	return nil
}

func addFileToZip(filename string, zipWriter *zip.Writer) error {
	// ZIPファイルに新しいファイルエントリを作成
	fileToZip, err := os.Open(filename)
	if err != nil {
		slog.Error(fmt.Sprintf("Can`t open %v", filename), "err", err)
		return err
	}
	defer fileToZip.Close()

	// ZIPファイル内のファイルエントリを作成
	fileInfo, err := fileToZip.Stat()
	if err != nil {
		slog.Error(fmt.Sprintf("Can`t get stat %v", filename), "err", err)
		return err
	}
	header, err := zip.FileInfoHeader(fileInfo)
	if err != nil {
		slog.Error(fmt.Sprintf("Can`t get read header %v", filename), "err", err)
		return err
	}

	// ZIPファイル内のパスを設定（ファイル名のみ）
	header.Name = filepath.Base(filename)
	header.Method = zip.Deflate

	// ZIPファイルにファイルを書き込む
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		slog.Error(fmt.Sprintf("Can`t create zip file header %v", filename), "err", err)
		return err
	}
	_, err = io.Copy(writer, fileToZip)
	if err != nil {
		slog.Error(fmt.Sprintf("Can`t add %v zip file", filename), "err", err)
		return err
	}

	return nil
}

func exportConfig() error {
	// 一時ディレクトリ名を生成する
	workDir, err := os.MkdirTemp("/tmp", "marmot-config")
	if err != nil {
		slog.Error("failed to creaate working directory", "err", err)
		return err
	}
	defer os.RemoveAll(workDir)

	// データベースと接続
	d, err := db.NewDatabase(etcdUrl)
	if err != nil {
		slog.Error("Failed to connect to etcd", "error", err)
		return err
	}

	// バージョンを書き込むこと
	// セットアップ時にバージョンを書き込むこと

	// ハイパーバイザー
	var hvs []types.Hypervisor
	err = d.GetHypervisors(&hvs)
	if err != nil {
		slog.Error("Failed to get hypervisor data", "error", err)
		return err
	}
	if err := writeJsonFile(filepath.Join(workDir, "marmot-hv.json"), hvs); err != nil {
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
	if err := writeJsonFile(filepath.Join(workDir, "marmot-os-temp.json"), osit); err != nil {
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
	if err := writeJsonFile(filepath.Join(workDir, "marmot-seq-data.json"), seqs); err != nil {
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
	if err := writeJsonFile(filepath.Join(workDir, "marmot-vm-data.json"), vms); err != nil {
		slog.Error("Failed to write data of virtual machines data", "error", err)
		return err
	}

	// 圧縮ファイルを作成
	if err := createZip(exportFilename, workDir); err != nil {
		slog.Error("Failed to create zip file", "error", err)
		return err
	}

	return nil
}
