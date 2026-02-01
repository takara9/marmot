package cmd

/*
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "仮想マシンの生成と起動",
	Long: `管理下のハイパーバイザーの一つに仮想マシンをスケジュールして生成と起動を実施します。
	デフォルトで 仮想マシンのスペック等が記述されたカレントディレクトリの cluster-config.yaml を使用します。`,
	Run: func(cmd *cobra.Command, args []string) {
		slog.Debug("===", "createCmd is called", "===")
		clusterConfig, err := config.ReadYamlClusterConfig(clusterConfigFilename)
		if err != nil {
			slog.Error("failed reading cluster-config file", "err", err.Error())
			return
		}

		_, _, _, err = m.CreateCluster(*clusterConfig)
		if err != nil {
			slog.Error("failed to create virtual machines", "err", err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.PersistentFlags().StringVarP(&clusterConfigFilename, "config", "c", "cluster-config.yaml", "")
}
*/
