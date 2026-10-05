package marmotd

import (
	"context"
	"testing"
)

// 実機で取得した `parted -m -s <dev> unit s print` の出力例。
const partedOutputSinglePartitionGPT = `BYT;
/dev/nbd0:2097152s:unknown:512:512:gpt:不明:;
1:2048s:2095103s:2093056s::p1:;
`

const partedOutputMultiplePartitionsGPT = `BYT;
/dev/nbd0:4194304s:unknown:512:512:gpt:不明:;
1:2048s:409599s:407552s::p1:;
2:409600s:819199s:409600s::p2:;
3:819200s:1228799s:409600s::p3:;
`

// Rocky 9 GenericCloud イメージ相当(bios_grub/ESP/boot/root)。
const partedOutputRocky9GPT = `BYT;
/dev/nbd0:33554432s:unknown:512:512:gpt:Virtio Block Device:;
1:2048s:6143s:4096s::p.legacy:bios_grub;
2:6144s:211967s:205824s:fat16:p.UEFI:boot, esp;
3:211968s:2273279s:2061312s:xfs:p.lxboot:bls_boot;
4:2273280s:33552383s:31279104s:xfs:p.lxroot:;
`

// qemu-img resize 直後、--fix 実行前に print すると、GPTバックアップヘッダの不整合警告が
// 先頭に出力されるが、パーティション一覧自体は正しく取得できる(issue #622, #737)。
const partedOutputWithResizeWarning = `警告: /dev/nbd0 で利用可能な領域の一部が利用されていません。GPT を修正して全ての領域を利用可能にするか(2097152 ブロック増えます)、このままで続行することができますが、どうしますか？ 
BYT;
/dev/nbd0:4194304s:unknown:512:512:gpt:不明:;
1:2048s:2095103s:2093056s::p1:;
`

const partedOutputDOSSinglePartition = `BYT;
/dev/sda:41943040s:scsi:512:512:msdos:Virtual Disk:;
1:2048s:41940991s:41938944s:ext4::boot;
`

func TestParseLastPartitionNumberFromPartedOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  string
		want    int
		wantErr bool
	}{
		{name: "single partition GPT (Ubuntu/Alpine相当)", output: partedOutputSinglePartitionGPT, want: 1},
		{name: "multiple partitions GPT", output: partedOutputMultiplePartitionsGPT, want: 3},
		{name: "Rocky 9 GenericCloud (4 partitions)", output: partedOutputRocky9GPT, want: 4},
		{name: "resize warning prefix is ignored", output: partedOutputWithResizeWarning, want: 1},
		{name: "dos partition table", output: partedOutputDOSSinglePartition, want: 1},
		{name: "empty output", output: "", wantErr: true},
		{name: "header only, no partitions", output: "BYT;\n/dev/nbd0:2097152s:unknown:512:512:gpt:不明:;\n", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseLastPartitionNumberFromPartedOutput(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result=%d)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLastPartitionNumberFromPartedOutput() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseLastPartitionNumberFromPartedOutput() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFindLastPartitionNumberInvalidDevice(t *testing.T) {
	t.Parallel()

	// 存在しないデバイスを指定した場合、parted がエラーになることを確認する。
	if _, err := findLastPartitionNumber(context.Background(), "/dev/marmot-test-nonexistent"); err == nil {
		t.Fatalf("expected error for nonexistent device")
	}
}

func TestDetectFilesystemTypeInvalidDevice(t *testing.T) {
	t.Parallel()

	// 存在しないデバイスを指定した場合、lsblk がエラーになることを確認する。
	if _, err := detectFilesystemType(context.Background(), "/dev/marmot-test-nonexistent"); err == nil {
		t.Fatalf("expected error for nonexistent device")
	}
}
