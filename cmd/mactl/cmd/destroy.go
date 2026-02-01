package cmd

/*
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "仮想マシンをシャットダウンして定義を削除します",
	Long: `管理下のハイパーバイザー上の仮想マシンのシャットダウンと定義の削除を実施します。
	デフォルトで 仮想マシンのスペック等が記述されたカレントディレクトリの cluster-config.yaml を使用します。`,
	Run: func(cmd *cobra.Command, args []string) {
		clusterConfig, err := config.ReadYamlClusterConfig(clusterConfigFilename)
		if err != nil {
			slog.Error("failed reading cluster-config file", "err", err.Error())
			return
		}
		_, _, _, err = m.DestroyCluster(*clusterConfig)
		if err != nil {
			slog.Error("failed to destroy virtual machines", "err", err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.PersistentFlags().StringVarP(&clusterConfigFilename, "cluster-config", "c", "cluster-config.yaml", "仮想サーバークラスタの構成ファイル")
}
*/
