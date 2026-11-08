package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/pkg/clusteradm"
)

var (
	clusterEtcdIP            string
	clusterUnderlayInterface string
	clusterDryRun            bool
	clusterForce             bool
	clusterMasterNode        string
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "marmot クラスタの構成を管理します",
	Long:  `marmotd を介さず etcd や marmotd.json を直接変更して、ノードをクラスタへ編入、離脱させます。root 権限が必要です。`,
}

var clusterInitCmd = &cobra.Command{
	Use:   "init",
	Short: "シングルノードの marmot をクラスタのコントロールノードへ変更します",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := clusteradm.InitOptions{
			EtcdIP:            clusterEtcdIP,
			UnderlayInterface: clusterUnderlayInterface,
			DryRun:            clusterDryRun,
		}
		if !clusterForce {
			opts.Confirm = confirmPrompt
		}
		return clusteradm.Init(context.Background(), clusteradm.DefaultEnv(), opts, os.Stdout)
	},
}

var clusterJoinCmd = &cobra.Command{
	Use:   "join CLUSTER-UUID",
	Short: "このノードを、マスターノードのクラスタへ編入します",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := clusteradm.JoinOptions{
			ClusterID:         args[0],
			MasterNode:        clusterMasterNode,
			UnderlayInterface: clusterUnderlayInterface,
			DryRun:            clusterDryRun,
		}
		if !clusterForce {
			opts.Confirm = confirmPrompt
		}
		return clusteradm.Join(context.Background(), clusteradm.DefaultEnv(), opts, os.Stdout)
	},
}

var clusterLeaveCmd = &cobra.Command{
	Use:   "leave CLUSTER-UUID",
	Short: "このノードを、クラスタから離脱させます（このノード上の VM は削除されます）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := clusteradm.LeaveOptions{ClusterID: args[0], DryRun: clusterDryRun}
		if !clusterForce {
			opts.Confirm = confirmPrompt
		}
		return clusteradm.Leave(context.Background(), clusteradm.DefaultEnv(), opts, os.Stdout)
	},
}

func confirmPrompt(string) bool {
	fmt.Print("続行しますか? [y/N]: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func init() {
	clusterInitCmd.Flags().StringVar(&clusterEtcdIP, "etcd-ip", "", "etcd が待ち受け、他ノードへ公開する IPv4 アドレス（既定: --underlay-interface の IPv4 アドレス）")
	clusterInitCmd.Flags().StringVar(&clusterUnderlayInterface, "underlay-interface", "", "ノード間のオーバーレイに使うインターフェース（既定: 現在の default_underlay_interface、未設定なら br0）")
	clusterInitCmd.Flags().BoolVar(&clusterDryRun, "dry-run", false, "変更内容を表示するだけで、何も変更しない")
	clusterInitCmd.Flags().BoolVar(&clusterForce, "force", false, "確認プロンプトを省略する")

	clusterJoinCmd.Flags().StringVar(&clusterMasterNode, "master-node", "", "マスターノードの IP アドレス（必須。etcd のポートは 2379）")
	_ = clusterJoinCmd.MarkFlagRequired("master-node")
	clusterJoinCmd.Flags().StringVar(&clusterUnderlayInterface, "underlay-interface", "", "ノード間のオーバーレイに使うインターフェース（既定: br0）")
	clusterJoinCmd.Flags().BoolVar(&clusterDryRun, "dry-run", false, "変更内容を表示するだけで、何も変更しない")
	clusterJoinCmd.Flags().BoolVar(&clusterForce, "force", false, "確認プロンプトを省略する")

	clusterLeaveCmd.Flags().BoolVar(&clusterDryRun, "dry-run", false, "削除される VM とその他の変更内容を表示するだけで、何も変更しない")
	clusterLeaveCmd.Flags().BoolVar(&clusterForce, "force", false, "確認プロンプトを省略する")

	clusterCmd.AddCommand(clusterInitCmd)
	clusterCmd.AddCommand(clusterLeaveCmd)
	clusterCmd.AddCommand(clusterJoinCmd)
	rootCmd.AddCommand(clusterCmd)
}
