package cmd

/*
var hvCmd = &cobra.Command{
	Use:   "hv",
	Short: "ハイパーバイザー（仮想マシンのホスト）を表示",
	Long:  `仮想マシンを動かすためのハイパーバイザーを表示します。`,
	Run: func(cmd *cobra.Command, args []string) {
		_, byteBody, _, err := m.ListHypervisors(nil)
		if err != nil {
			slog.Error("list vms", "err", err)
			return
		}
		status := [3]string{"HLT", "ERR", "RUN"}
		dec := json.NewDecoder(strings.NewReader(string(byteBody)))
		dec.Token()
		fmt.Printf("%-10s %-3v %-15v %-8v  %-12v   %-12v", "HV-NAME", "ONL", "IPaddr", "VCPU", "RAM(MB)", "Storage(GB)")
		fmt.Printf("\n")
		for dec.More() {
			var hv api.Hypervisor
			err := dec.Decode(&hv)
			if err != nil {
				slog.Error("reading hypervisors status", "err", err)
			}
			fmt.Printf("%-10s %-3v %-15v %4d/%-4d %6d/%-6d  ",
				hv.NodeName, status[*hv.Status], *hv.IpAddr, *hv.FreeCpu, hv.Cpu, *hv.FreeMemory, *hv.Memory)
			for _, v := range *hv.StgPool {
				fmt.Printf("%v(%v): %5d/%-5d ", *v.VolGroup, *v.Type, *v.FreeCap, *v.VgCap)
			}
			fmt.Printf("\n")
		}
		dec.Token()
	},
}

func init() {
	rootCmd.AddCommand(hvCmd)
}
*/
