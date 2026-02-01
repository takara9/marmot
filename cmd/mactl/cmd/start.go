package cmd

/*
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "停止中の仮想マシンを開始します。",
	Long: `stop で停止された仮想マシンの活動を再開します。
	デフォルトで 仮想マシンのスペック等が記述されたカレントディレクトリの cluster-config.yaml を使用します。`,
	Run: func(cmd *cobra.Command, args []string) {
		clusterConfig, err := config.ReadYamlClusterConfig(clusterConfigFilename)
		if err != nil {
			slog.Error("failed reading cluster-config file", "err", err.Error())
			return
		}

		_, _, _, err = m.StartCluster(*clusterConfig)
		if err != nil {
			slog.Error("failed to start virtual machines", "err", err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.PersistentFlags().StringVarP(&clusterConfigFilename, "cluster-config", "c", "cluster-config.yaml", "仮想サーバークラスタの構成ファイル")
}
*/
