package marmotd

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"gopkg.in/yaml.v3"
)

func TestSanitizeWindowsHostname(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"win2k22-vm", "win2k22-vm"},
		{"  win_2022.vm ", "win-2022-vm"},
		{"a-very-long-server-name", "a-very-long-ser"},
		{"abcdefghijklmn-opq", "abcdefghijklmn"},
		{"--", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := sanitizeWindowsHostname(tt.in); got != tt.want {
			t.Errorf("sanitizeWindowsHostname(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildCloudbaseInitMetaData(t *testing.T) {
	t.Parallel()
	raw, err := buildCloudbaseInitMetaData("pass!", "ssh-rsa AAA one\n\nssh-ed25519 BBB two\n", "inst-1", "win_vm")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got["uuid"] != "inst-1" || got["hostname"] != "win-vm" || got["admin_pass"] != "pass!" {
		t.Fatalf("unexpected meta_data: %v", got)
	}
	keys, _ := got["public_keys"].(map[string]any)
	if len(keys) != 2 || keys["key-0"] != "ssh-rsa AAA one" || keys["key-1"] != "ssh-ed25519 BBB two" {
		t.Fatalf("unexpected public_keys: %v", keys)
	}
}

func TestBuildCloudbaseInitMetaDataOmitsEmptyValues(t *testing.T) {
	t.Parallel()
	raw, err := buildCloudbaseInitMetaData("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["uuid"] != "iid-local01" {
		t.Fatalf("expected only the default uuid, got %v", got)
	}
}

func TestBuildCloudbaseInitUserData(t *testing.T) {
	t.Parallel()

	type parsedConfig struct {
		Users []struct {
			Name   string `yaml:"name"`
			Groups string `yaml:"groups"`
			Passwd string `yaml:"passwd"`
		} `yaml:"users"`
		RunCmd []string `yaml:"runcmd"`
	}
	parse := func(t *testing.T, out string) parsedConfig {
		t.Helper()
		if !strings.HasPrefix(out, "#cloud-config\n") {
			t.Fatalf("missing #cloud-config header: %q", out)
		}
		var cfg parsedConfig
		if err := yaml.Unmarshal([]byte(out), &cfg); err != nil {
			t.Fatalf("invalid yaml: %v", err)
		}
		return cfg
	}

	// 鍵なし・ユーザーなし("root" は除外される): RDP の runcmd のみ。
	out, err := buildCloudbaseInitUserData("pw", "", []string{"root", " "})
	if err != nil {
		t.Fatal(err)
	}
	cfg := parse(t, out)
	if len(cfg.Users) != 0 || len(cfg.RunCmd) != len(enableRDPCommands)+1 {
		t.Fatalf("unexpected user_data: %q", out)
	}
	assertRunCmdExitsOnlyAtEnd(t, cfg.RunCmd)

	// 鍵なし・ユーザーあり: 既存動作どおり Administrators のユーザーを作成し、sshd の設定は含まない。
	out, err = buildCloudbaseInitUserData("p: w #1", "", []string{"root", "alice"})
	if err != nil {
		t.Fatal(err)
	}
	cfg = parse(t, out)
	if len(cfg.Users) != 1 {
		t.Fatalf("root must be excluded, got %+v", cfg.Users)
	}
	if u := cfg.Users[0]; u.Name != "alice" || u.Groups != "Administrators" || u.Passwd != "p: w #1" {
		t.Fatalf("unexpected user: %+v", u)
	}
	if len(cfg.RunCmd) != len(enableRDPCommands)+1 || cfg.RunCmd[0] != enableRDPCommands[0] {
		t.Fatalf("runcmd must round-trip unchanged: %v", cfg.RunCmd)
	}
	assertRunCmdExitsOnlyAtEnd(t, cfg.RunCmd)

	// 鍵あり・ユーザーなし("root" のみは未指定扱い): RDP に続けて sshd の設定が入る。
	out, err = buildCloudbaseInitUserData("pw", "ssh-rsa AAA one\nssh-ed25519 BBB two\n", []string{"root"})
	if err != nil {
		t.Fatal(err)
	}
	cfg = parse(t, out)
	if len(cfg.Users) != 0 {
		t.Fatalf("unexpected users: %+v", cfg.Users)
	}
	wantLen := len(enableRDPCommands) + 2 + len(sshdFinishCommands) + 1
	if len(cfg.RunCmd) != wantLen || cfg.RunCmd[0] != enableRDPCommands[0] {
		t.Fatalf("unexpected runcmd (len=%d, want %d): %v", len(cfg.RunCmd), wantLen, cfg.RunCmd)
	}
	if got, want := cfg.RunCmd[len(cfg.RunCmd)-2], sshdFinishCommands[len(sshdFinishCommands)-1]; got != want {
		t.Fatalf("sshd commands must come last before exit: %v", cfg.RunCmd)
	}
	assertRunCmdExitsOnlyAtEnd(t, cfg.RunCmd)

	// 鍵あり・ユーザーあり: 未実装エラー。
	if _, err := buildCloudbaseInitUserData("pw", "ssh-rsa AAA", []string{"alice"}); err == nil || !strings.Contains(err.Error(), "未実装") {
		t.Fatalf("expected not-implemented error, got %v", err)
	}
}

func decodePowerShellCommand(t *testing.T, cmd string) string {
	t.Helper()
	const prefix = "-EncodedCommand "
	i := strings.Index(cmd, prefix)
	if i < 0 {
		t.Fatalf("not an encoded powershell command: %q", cmd)
	}
	raw, err := base64.StdEncoding.DecodeString(cmd[i+len(prefix):])
	if err != nil || len(raw)%2 != 0 {
		t.Fatalf("invalid base64 UTF-16LE: %v", err)
	}
	u16 := make([]uint16, len(raw)/2)
	for j := range u16 {
		u16[j] = uint16(raw[2*j]) | uint16(raw[2*j+1])<<8
	}
	return string(utf16.Decode(u16))
}

func TestSSHDAuthorizedKeysCommands(t *testing.T) {
	t.Parallel()
	cmds, err := sshdAuthorizedKeysCommands([]string{"ssh-rsa AAA one", "ssh-ed25519 BBB two"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2+len(sshdFinishCommands) {
		t.Fatalf("unexpected commands: %v", cmds)
	}

	// 最初の鍵は上書き、2つ目以降は追記。鍵は base64 で埋め込まれ、元の鍵に復元できる。
	for i, tc := range []struct{ method, key string }{
		{"WriteAllText", "ssh-rsa AAA one"},
		{"AppendAllText", "ssh-ed25519 BBB two"},
	} {
		script := decodePowerShellCommand(t, cmds[i])
		if !strings.Contains(script, "[IO.File]::"+tc.method+"(") || !strings.Contains(script, `administrators_authorized_keys`) {
			t.Fatalf("unexpected script[%d]: %s", i, script)
		}
		keyB64 := base64.StdEncoding.EncodeToString([]byte(tc.key + "\n"))
		if !strings.Contains(script, "'"+keyB64+"'") {
			t.Fatalf("key not embedded in script[%d]: %s", i, script)
		}
	}

	joined := strings.Join(sshdFinishCommands, "\n")
	for _, want := range []string{`/inheritance:r`, `"*S-1-5-18:F"`, `"*S-1-5-32-544:F"`, `localport=22`, `net start sshd`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("%q not found in %q", want, joined)
		}
	}
	assertRunCmdExitsOnlyAtEnd(t, append(cmds, runcmdExitCommand))

	if _, err := sshdAuthorizedKeysCommands([]string{strings.Repeat("A", 4000)}); err == nil {
		t.Fatal("expected error for too long key")
	}
}

// isoFiles は xorriso で ISO 内のファイル一覧とボリュームラベルを返す。
func isoFiles(t *testing.T, iso string) (label string, files string) {
	t.Helper()
	if _, err := exec.LookPath("xorriso"); err != nil {
		t.Skip("xorriso not available")
	}
	out, err := exec.Command("xorriso", "-indev", iso, "-find", "/", "-type", "f").CombinedOutput()
	if err != nil {
		t.Fatalf("xorriso find failed: %v\n%s", err, out)
	}
	files = string(out)
	pvd, err := exec.Command("xorriso", "-indev", iso, "-pvd_info").CombinedOutput()
	if err != nil {
		t.Fatalf("xorriso pvd_info failed: %v\n%s", err, pvd)
	}
	for _, line := range strings.Split(string(pvd), "\n") {
		if strings.HasPrefix(line, "Volume Id") {
			label = strings.TrimSpace(strings.TrimPrefix(line, "Volume Id    :"))
		}
	}
	return label, files
}

func TestGenerateCloudbaseInitISO(t *testing.T) {
	if _, err := exec.LookPath("genisoimage"); err != nil {
		t.Skip("genisoimage not available")
	}
	path := filepath.Join(t.TempDir(), "isos", "srv")
	iso, err := GenerateCloudbaseInitISO(path, "pw", "", []string{"alice"}, "srv-1", "win-vm", testNICs())
	if err != nil {
		t.Fatal(err)
	}
	if iso != filepath.Join(path, "cloud-init.iso") {
		t.Fatalf("unexpected iso path %q", iso)
	}
	label, files := isoFiles(t, iso)
	if label != "config-2" {
		t.Fatalf("volume label = %q, want config-2", label)
	}
	for _, want := range []string{"/openstack/latest/meta_data.json", "/openstack/latest/user_data", "/openstack/latest/network_data.json"} {
		if !strings.Contains(files, want) {
			t.Fatalf("%s not found in ISO:\n%s", want, files)
		}
	}
}

// 公開鍵のみ(ユーザー未指定)なら ISO を生成でき、公開鍵とユーザーの同時指定は ISO 生成前にエラーになる。
func TestGenerateCloudbaseInitISOWithSSHKey(t *testing.T) {
	if _, err := exec.LookPath("genisoimage"); err != nil {
		t.Skip("genisoimage not available")
	}
	path := filepath.Join(t.TempDir(), "iso")
	if _, err := GenerateCloudbaseInitISO(path, "", "ssh-rsa AAA", nil, "srv-1", "win-vm", nil); err != nil {
		t.Fatalf("key only must succeed: %v", err)
	}
	path = filepath.Join(t.TempDir(), "iso2")
	if _, err := GenerateCloudbaseInitISO(path, "", "ssh-rsa AAA", []string{"alice"}, "srv-1", "win-vm", nil); err == nil {
		t.Fatal("key with user must fail")
	}
	if _, err := os.Stat(filepath.Join(path, "cloud-init.iso")); err == nil {
		t.Fatal("ISO must not be created on error")
	}
}

// Windows モジュールは ConfigDrive 形式、Linux モジュールは従来の cidata 形式の ISO を生成する。
func TestServerImageModuleGenerateISOByOS(t *testing.T) {
	if _, err := exec.LookPath("genisoimage"); err != nil {
		t.Skip("genisoimage not available")
	}
	tests := []struct {
		name      string
		mod       serverImageModule
		wantLabel string
		wantFile  string
	}{
		{"windows2022", serverImageModuleWindows2022, "config-2", "/openstack/latest/meta_data.json"},
		{"windows2025", serverImageModuleWindows2025, "config-2", "/openstack/latest/meta_data.json"},
		{"ubuntu24.04", serverImageModuleUbuntu2404, "cidata", "/user-data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "iso")
			iso, err := tt.mod.GenerateCloudInitISO(path, "pw", "", []string{"alice"}, (*api.ServerAnsible)(nil), "srv-1", "host1", testNICs())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(iso); err != nil {
				t.Fatal(err)
			}
			label, files := isoFiles(t, iso)
			if label != tt.wantLabel || !strings.Contains(files, tt.wantFile) {
				t.Fatalf("label=%q files=\n%s", label, files)
			}
		})
	}
}

func testNICs() []api.NetworkInterface {
	return []api.NetworkInterface{
		{
			Mac:         util.StringPtr("7a:c1:68:70:53:6a"),
			Address:     util.StringPtr("192.168.1.188"),
			Netmasklen:  util.IntPtrInt(24),
			IpGateway:   util.StringPtr("192.168.1.1"),
			Nameservers: &api.Nameservers{Addresses: &[]string{"192.168.1.8"}},
		},
		{
			Mac:         util.StringPtr("86:85:53:98:fd:b9"),
			Address:     util.StringPtr("10.245.0.20"),
			Netmask:     util.StringPtr("255.255.0.0"),
			Nameservers: &api.Nameservers{Addresses: &[]string{"10.245.0.1"}},
		},
	}
}

func TestBuildCloudbaseInitNetworkData(t *testing.T) {
	t.Parallel()
	raw, err := buildCloudbaseInitNetworkData(testNICs())
	if err != nil {
		t.Fatal(err)
	}
	var got cloudbaseInitNetworkData
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Links) != 2 || len(got.Networks) != 2 {
		t.Fatalf("expected 2 links and 2 networks: %s", raw)
	}
	if got.Links[0].MacAddress != "7a:c1:68:70:53:6a" || got.Links[1].MacAddress != "86:85:53:98:fd:b9" {
		t.Fatalf("unexpected links: %+v", got.Links)
	}
	hb, mgmt := got.Networks[0], got.Networks[1]
	if hb.Link != got.Links[0].ID || hb.Type != "ipv4" || hb.IPAddress != "192.168.1.188" || hb.Netmask != "255.255.255.0" {
		t.Fatalf("unexpected host-bridge network: %+v", hb)
	}
	if len(hb.Routes) != 1 || hb.Routes[0] != (cloudbaseInitRoute{Network: "0.0.0.0", Netmask: "0.0.0.0", Gateway: "192.168.1.1"}) {
		t.Fatalf("unexpected host-bridge routes: %+v", hb.Routes)
	}
	if len(hb.Services) != 1 || hb.Services[0] != (cloudbaseInitDNSSvc{Type: "dns", Address: "192.168.1.8"}) {
		t.Fatalf("unexpected host-bridge dns: %+v", hb.Services)
	}
	if mgmt.IPAddress != "10.245.0.20" || mgmt.Netmask != "255.255.0.0" || len(mgmt.Routes) != 0 {
		t.Fatalf("unexpected mgmt network: %+v", mgmt)
	}
}

func TestBuildCloudbaseInitNetworkDataRoutes(t *testing.T) {
	t.Parallel()
	to1, via1 := "default", "10.0.0.1"
	to2, via2 := "172.16.0.0/12", "10.0.0.254"
	to3, via3 := "bad", "10.0.0.2"
	nic := api.NetworkInterface{
		Mac:        util.StringPtr("aa:bb:cc:dd:ee:ff"),
		Address:    util.StringPtr("10.0.0.5"),
		Netmasklen: util.IntPtrInt(24),
		IpGateway:  util.StringPtr("10.0.0.99"),
		Routes:     &[]api.Route{{To: &to1, Via: &via1}, {To: &to2, Via: &via2}, {To: &to3, Via: &via3}},
	}
	routes := nicRoutes(nic)
	want := []cloudbaseInitRoute{
		{Network: "0.0.0.0", Netmask: "0.0.0.0", Gateway: "10.0.0.1"},
		{Network: "172.16.0.0", Netmask: "255.240.0.0", Gateway: "10.0.0.254"},
	}
	if len(routes) != len(want) || routes[0] != want[0] || routes[1] != want[1] {
		t.Fatalf("routes = %+v, want %+v", routes, want)
	}
}

func TestBuildCloudbaseInitNetworkDataSkipsUnconfiguredNICs(t *testing.T) {
	t.Parallel()
	raw, err := buildCloudbaseInitNetworkData([]api.NetworkInterface{
		{Mac: util.StringPtr("aa:bb:cc:dd:ee:01")},                                                                 // アドレスなし(DHCP)
		{Address: util.StringPtr("10.0.0.5"), Netmasklen: util.IntPtrInt(24)},                                      // MAC なし
		{Mac: util.StringPtr("aa:bb:cc:dd:ee:03"), Address: util.StringPtr("10.0.0.6")},                            // ネットマスクなし
		{Mac: util.StringPtr("aa:bb:cc:dd:ee:04"), Address: util.StringPtr("bad"), Netmasklen: util.IntPtrInt(24)}, // 不正なアドレス
	})
	if err != nil || raw != nil {
		t.Fatalf("expected nil network data, got %s (err=%v)", raw, err)
	}
}

// RDP 有効化は、レジストリ(接続許可)とファイアウォール(ロケール非依存のルールグループ指定)の
// 両方を含み、途中で exit しない。
func TestEnableRDPCommands(t *testing.T) {
	t.Parallel()
	joined := strings.Join(enableRDPCommands, "\n")
	for _, want := range []string{"fDenyTSConnections /t REG_DWORD /d 0", `group="@FirewallAPI.dll,-28752"`, "TermService", `name="marmot-RDP-3389"`, "localport=3389"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("%q not found in %q", want, joined)
		}
	}
	assertRunCmdExitsOnlyAtEnd(t, append(append([]string{}, enableRDPCommands...), runcmdExitCommand))
}

// cloudbase-init は runcmd の全エントリを1つのバッチファイルにして実行するため、
// 途中のエントリに exit があると以降のエントリが実行されない。exit は最後の1つだけであること。
func assertRunCmdExitsOnlyAtEnd(t *testing.T, cmds []string) {
	t.Helper()
	if len(cmds) == 0 || cmds[len(cmds)-1] != "exit /b 0" {
		t.Fatalf("last runcmd entry must be 'exit /b 0': %v", cmds)
	}
	for i, c := range cmds[:len(cmds)-1] {
		if !strings.HasPrefix(c, "powershell.exe") && strings.Contains(strings.ToLower(c), "exit") {
			t.Fatalf("runcmd[%d] must not contain exit (it would skip the following entries): %q", i, c)
		}
	}
}
