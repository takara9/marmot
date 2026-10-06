//go:build linux
// +build linux

package util

import (
	"testing"
)

// 実機で取得した `parted -m -s <dev> unit s print` の出力例。
const partedOutputSinglePartitionGPT = `BYT;
/dev/nbd0:2097152s:unknown:512:512:gpt:不明:;
1:2048s:2095103s:2093056s::p1:;
`

// Ubuntu 24.04 cloud image 実機相当。ルートパーティションの番号は「1」だが、
// bios_grub/ESP/boot には番号14/15/16(ルートより大きい番号)が割り当てられている。
// ルートパーティションはサイズとしては最大だが、番号としては最大ではない(issue #622, #737)。
const partedOutputUbuntu2404GPT = `BYT;
/dev/nbd0:7340032s:unknown:512:512:gpt:不明:;
14:2048s:10239s:8192s:::bios_grub;
15:10240s:227327s:217088s:fat32::boot, esp;
16:227328s:2097152s:1869825s:ext4::bls_boot;
1:2099200s:7339998s:5240799s:ext4::;
`

// Rocky 9 GenericCloud イメージ実機相当(bios_grub/ESP/boot/root)。
const partedOutputRocky9GPT = `BYT;
/dev/nbd0:33554432s:unknown:512:512:gpt:Virtio Block Device:;
1:2048s:6143s:4096s::p.legacy:bios_grub;
2:6144s:211967s:205824s:fat16:p.UEFI:boot, esp;
3:211968s:2273279s:2061312s:xfs:p.lxboot:bls_boot;
4:2273280s:33552383s:31279104s:xfs:p.lxroot:;
`

// qemu-img resize 直後、--fix 実行前に print すると、GPTバックアップヘッダの不整合警告が
// 先頭に出力されるが、パーティション一覧自体は正しく取得できる(issue #622)。
const partedOutputWithResizeWarning = `警告: /dev/nbd0 で利用可能な領域の一部が利用されていません。GPT を修正して全ての領域を利用可能にするか(2097152 ブロック増えます)、このままで続行することができますが、どうしますか？ 
BYT;
/dev/nbd0:4194304s:unknown:512:512:gpt:不明:;
1:2048s:2095103s:2093056s::p1:;
`

// マウント対象は「最大サイズのパーティション(= ルートファイルシステム)」。
// Ubuntu実機のように、ルートパーティションの番号が最大番号ではないケースを含めて検証する。
func TestParseRootPartitionNumberFromPartedOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  string
		want    int
		wantErr bool
	}{
		{name: "single partition GPT (Alpine相当)", output: partedOutputSinglePartitionGPT, want: 1},
		{name: "Ubuntu 24.04実機相当(ルートは番号1だが最大サイズ)", output: partedOutputUbuntu2404GPT, want: 1},
		{name: "Rocky 9 GenericCloud実機相当(ルートは番号4で最大サイズ)", output: partedOutputRocky9GPT, want: 4},
		{name: "resize warning prefix is ignored", output: partedOutputWithResizeWarning, want: 1},
		{name: "empty output", output: "", wantErr: true},
		{name: "header only, no partitions", output: "BYT;\n/dev/nbd0:2097152s:unknown:512:512:gpt:不明:;\n", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseRootPartitionNumberFromPartedOutput(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result=%d)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRootPartitionNumberFromPartedOutput() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseRootPartitionNumberFromPartedOutput() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFindRootPartitionNumberInvalidDevice(t *testing.T) {
	t.Parallel()

	// 存在しないデバイスを指定した場合、parted がエラーになることを確認する。
	if _, err := findRootPartitionNumber("/dev/marmot-test-nonexistent"); err == nil {
		t.Fatalf("expected error for nonexistent device")
	}
}
