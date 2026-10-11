package marmotd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

	// 追加ユーザーがなくても、RDP を有効にする runcmd は常に含まれる("root" は除外される)。
	noUsers, err := buildCloudbaseInitUserData("pw", "ssh-rsa AAA", []string{"root", " "})
	if err != nil {
		t.Fatal(err)
	}
	var noUsersCfg struct {
		Users  []map[string]any `yaml:"users"`
		RunCmd []string         `yaml:"runcmd"`
	}
	if err := yaml.Unmarshal([]byte(noUsers), &noUsersCfg); err != nil {
		t.Fatalf("invalid yaml: %v", err)
	}
	if len(noUsersCfg.Users) != 0 || len(noUsersCfg.RunCmd) != len(enableRDPCommands) {
		t.Fatalf("unexpected user_data: %q", noUsers)
	}

	out, err := buildCloudbaseInitUserData("p: w #1", "ssh-rsa AAA", []string{"root", "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "#cloud-config\n") {
		t.Fatalf("missing #cloud-config header: %q", out)
	}
	var parsed struct {
		Users []struct {
			Name   string   `yaml:"name"`
			Groups string   `yaml:"groups"`
			Passwd string   `yaml:"passwd"`
			Keys   []string `yaml:"ssh_authorized_keys"`
		} `yaml:"users"`
		RunCmd []string `yaml:"runcmd"`
	}
	if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid yaml: %v", err)
	}
	if len(parsed.Users) != 1 {
		t.Fatalf("root must be excluded, got %+v", parsed.Users)
	}
	u := parsed.Users[0]
	if len(parsed.RunCmd) != len(enableRDPCommands) || parsed.RunCmd[0] != enableRDPCommands[0] {
		t.Fatalf("runcmd must round-trip unchanged: %v", parsed.RunCmd)
	}
	if u.Name != "alice" || u.Groups != "Administrators" || u.Passwd != "p: w #1" || len(u.Keys) != 1 || u.Keys[0] != "ssh-rsa AAA" {
		t.Fatalf("unexpected user: %+v", u)
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
	iso, err := GenerateCloudbaseInitISO(path, "pw", "ssh-rsa AAA", []string{"alice"}, "srv-1", "win-vm", testNICs())
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
			iso, err := tt.mod.GenerateCloudInitISO(path, "pw", "ssh-rsa AAA", []string{"alice"}, (*api.ServerAnsible)(nil), "srv-1", "host1", testNICs())
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
// 両方を含み、最後は終了コード 0 で終わる。
func TestEnableRDPCommands(t *testing.T) {
	t.Parallel()
	joined := strings.Join(enableRDPCommands, "\n")
	for _, want := range []string{"fDenyTSConnections /t REG_DWORD /d 0", `group="@FirewallAPI.dll,-28752"`, "TermService"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("%q not found in %q", want, joined)
		}
	}
	if last := enableRDPCommands[len(enableRDPCommands)-1]; !strings.HasSuffix(last, "exit /b 0") {
		t.Fatalf("last command must exit 0: %q", last)
	}
}
