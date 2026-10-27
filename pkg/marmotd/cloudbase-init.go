package marmotd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/takara9/marmot/api"
	"gopkg.in/yaml.v3"
)

// netbiosHostNameMaxLen は Windows のコンピューター名(NetBIOS 名)の最大長。
const netbiosHostNameMaxLen = 15

// enableRDPCommands は、リモートデスクトップ(RDP)の接続受け入れを有効にするコマンド列。
// cloudbase-init の runcmd は cmd.exe で実行される。
//   - ファイアウォールのルールグループは、表示名がロケールで変わる(日本語では「リモート デスクトップ」)ため、
//     ロケールに依存しないリソース参照(@FirewallAPI.dll,-28752)で指定する。
//   - ただし実機(Windows Server 2025)では、このグループ指定による有効化が定義済みルールに
//     反映されず Enabled=False のまま残ることを確認した。そのため、グループ指定のルールが
//     有効化できない場合でも接続できるよう、ロケールや既存ルール定義に依存しない TCP 3389 への
//     明示的な受信許可ルールを追加で作成する(delete は初回実行時に対象が無く失敗するが、
//     前行の失敗で後続が止まることはないため、そのままで問題ない)。
//   - NLA(ネットワークレベル認証)は Windows Server の既定(有効)のまま変更しない。
//   - cloudbase-init は、runcmd の全エントリを1つのバッチファイルにまとめて実行する。
//     そのため、エントリの途中で exit を実行すると、以降のエントリが実行されなくなる。
//     終了コードを 0 にする exit /b 0 は、全エントリの最後に1回だけ置く(runcmdExitCommand)。
var enableRDPCommands = []string{
	`reg add "HKLM\SYSTEM\CurrentControlSet\Control\Terminal Server" /v fDenyTSConnections /t REG_DWORD /d 0 /f`,
	`netsh advfirewall firewall set rule group="@FirewallAPI.dll,-28752" new enable=Yes`,
	`netsh advfirewall firewall delete rule name="marmot-RDP-3389"`,
	`netsh advfirewall firewall add rule name="marmot-RDP-3389" dir=in action=allow protocol=TCP localport=3389 profile=any`,
	`sc config TermService start= auto`,
	`net start TermService`,
}

// runcmdExitCommand は runcmd の最後に置くコマンド。net start は対象が既に起動していると
// エラーを返すため、スクリプト全体が失敗扱いにならないよう終了コード 0 で終える。
const runcmdExitCommand = `exit /b 0`

// cmdLineMaxLen は cmd.exe の1コマンド行の最大長(8191 文字)に対する、runcmd の1エントリの上限。
const cmdLineMaxLen = 8000

// administratorsAuthorizedKeysPath は、Administrators グループのユーザー用の sshd 公開鍵ファイル。
const administratorsAuthorizedKeysPath = `C:\ProgramData\ssh\administrators_authorized_keys`

// sshdFinishCommands は、公開鍵の配置後に実行する sshd 向けのコマンド列。
// OpenSSH Server のインストール・自動起動設定・既定シェルの設定はベースイメージ作成時に行われている前提で、
// ここでは鍵ファイルの権限設定、ファイアウォール開放、sshd の起動のみを行う。
//   - administrators_authorized_keys は SYSTEM と Administrators 以外に権限があると sshd に無視されるため、
//     継承を無効化して両者のみに権限を残す。ロケールに依存しないよう、アカウント名ではなく既知 SID
//     (SYSTEM は *S-1-5-18、Administrators は *S-1-5-32-544)で指定する。
//   - runcmd の全エントリは1つのバッチファイルになるため、ここでは exit を実行しない(runcmdExitCommand 参照)。
var sshdFinishCommands = []string{
	`icacls.exe ` + administratorsAuthorizedKeysPath + ` /inheritance:r /grant:r "*S-1-5-18:F" /grant:r "*S-1-5-32-544:F"`,
	`netsh advfirewall firewall delete rule name="marmot-SSH-22"`,
	`netsh advfirewall firewall add rule name="marmot-SSH-22" dir=in action=allow protocol=TCP localport=22 profile=any`,
	`sc config sshd start= auto`,
	`net start sshd`,
}

// cloudbaseInitMetaData は OpenStack ConfigDrive の openstack/latest/meta_data.json の内容。
type cloudbaseInitMetaData struct {
	UUID       string            `json:"uuid"`
	Hostname   string            `json:"hostname,omitempty"`
	Name       string            `json:"name,omitempty"`
	AdminPass  string            `json:"admin_pass,omitempty"`
	PublicKeys map[string]string `json:"public_keys,omitempty"`
}

// cloudbaseInitNetworkData は OpenStack ConfigDrive の openstack/latest/network_data.json の内容。
type cloudbaseInitNetworkData struct {
	Links    []cloudbaseInitLink    `json:"links"`
	Networks []cloudbaseInitNetwork `json:"networks"`
}

type cloudbaseInitLink struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	MacAddress string `json:"ethernet_mac_address"`
}

type cloudbaseInitNetwork struct {
	ID        string                `json:"id"`
	Link      string                `json:"link"`
	Type      string                `json:"type"`
	IPAddress string                `json:"ip_address"`
	Netmask   string                `json:"netmask"`
	Routes    []cloudbaseInitRoute  `json:"routes,omitempty"`
	Services  []cloudbaseInitDNSSvc `json:"services,omitempty"`
}

type cloudbaseInitRoute struct {
	Network string `json:"network"`
	Netmask string `json:"netmask"`
	Gateway string `json:"gateway"`
}

type cloudbaseInitDNSSvc struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

// GenerateCloudbaseInitISO は、Windows VM の cloudbase-init 向けに OpenStack ConfigDrive 形式
// (ボリュームラベル config-2、openstack/latest/meta_data.json と user_data)の ISO を生成する。
//
// ホスト名・管理者パスワード・SSH 公開鍵は meta_data.json 経由で、追加ユーザーと RDP の有効化は
// user_data(cloud-config)経由で、静的 IP は network_data.json 経由で cloudbase-init の標準プラグインが
// 適用する。ISO のファイル名は Linux 向けと同じ cloud-init.iso とし、CD-ROM のアタッチ処理を共通に保つ。
func GenerateCloudbaseInitISO(path, password, sshKey string, usernames []string, instanceID, hostname string, nics []api.NetworkInterface) (string, error) {
	tempDir, err := os.MkdirTemp("", "cloudbase-init-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	metaData, err := buildCloudbaseInitMetaData(password, sshKey, instanceID, hostname)
	if err != nil {
		return "", err
	}
	userData, err := buildCloudbaseInitUserData(password, sshKey, usernames)
	if err != nil {
		return "", err
	}

	networkData, err := buildCloudbaseInitNetworkData(nics)
	if err != nil {
		return "", err
	}

	dataDir := filepath.Join(tempDir, "openstack", "latest")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config drive dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "meta_data.json"), metaData, 0644); err != nil {
		slog.Error("cloudbase-initの作成中に、meta_data.jsonの書き込みに失敗", "error", err)
		return "", fmt.Errorf("failed to write meta_data.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "user_data"), []byte(userData), 0644); err != nil {
		slog.Error("cloudbase-initの作成中に、user_dataの書き込みに失敗", "error", err)
		return "", fmt.Errorf("failed to write user_data: %v", err)
	}

	if networkData != nil {
		if err := os.WriteFile(filepath.Join(dataDir, "network_data.json"), networkData, 0644); err != nil {
			slog.Error("cloudbase-initの作成中に、network_data.jsonの書き込みに失敗", "error", err)
			return "", fmt.Errorf("failed to write network_data.json: %v", err)
		}
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		slog.Error("cloudbase-initの作成中に、ディレクトリの作成に失敗", "error", err)
		return "", fmt.Errorf("failed to create directory: %v", err)
	}
	isoPath := filepath.Join(path, "cloud-init.iso")
	slog.Debug("cloudbase-init ISOの生成を開始", "isoPath", isoPath)
	cmd := exec.Command("genisoimage", "-output", isoPath, "-volid", "config-2", "-joliet", "-rock", tempDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Error("cloudbase-initの作成中に、ISOの生成に失敗", "error", err, "output", string(out))
		return "", fmt.Errorf("failed to generate ISO: %v", err)
	}
	return isoPath, nil
}

func buildCloudbaseInitMetaData(password, sshKey, instanceID, hostname string) ([]byte, error) {
	if strings.TrimSpace(instanceID) == "" {
		instanceID = "iid-local01"
	}
	name := sanitizeWindowsHostname(hostname)
	md := cloudbaseInitMetaData{
		UUID:      instanceID,
		Hostname:  name,
		Name:      name,
		AdminPass: password,
	}
	keys := splitSSHKeys(sshKey)
	if len(keys) > 0 {
		md.PublicKeys = make(map[string]string, len(keys))
		for i, k := range keys {
			md.PublicKeys[fmt.Sprintf("key-%d", i)] = k
		}
	}
	data, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to build meta_data.json: %v", err)
	}
	return data, nil
}

// sshdAuthorizedKeysCommands は、公開鍵を administrators_authorized_keys に書き込む runcmd のコマンド列を返す。
// runcmd は cmd.exe で実行されるため、鍵は base64 で埋め込んだ PowerShell スクリプトを -EncodedCommand で渡す。
// cmd.exe の行長制限を避けるため、1行につき1つの鍵を書き込む(最初の1行が上書き、以降は追記)。
func sshdAuthorizedKeysCommands(keys []string) ([]string, error) {
	cmds := make([]string, 0, len(keys)+len(sshdFinishCommands))
	for i, key := range keys {
		write := "AppendAllText"
		if i == 0 {
			write = "WriteAllText"
		}
		keyB64 := base64.StdEncoding.EncodeToString([]byte(key + "\n"))
		script := fmt.Sprintf(
			"$d='C:\\ProgramData\\ssh'; New-Item -ItemType Directory -Force -Path $d | Out-Null; "+
				"$k=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')); "+
				"[IO.File]::%s(\"$d\\administrators_authorized_keys\", $k)", keyB64, write)
		cmd := "powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand " + encodePowerShellCommand(script)
		if len(cmd) > cmdLineMaxLen {
			return nil, fmt.Errorf("公開鍵 %d 番目が長すぎて Windows のコマンド行に収まりません", i+1)
		}
		cmds = append(cmds, cmd)
	}
	return append(cmds, sshdFinishCommands...), nil
}

// encodePowerShellCommand は PowerShell の -EncodedCommand 用に、スクリプトを UTF-16LE の base64 にする。
func encodePowerShellCommand(script string) string {
	u16 := utf16.Encode([]rune(script))
	buf := make([]byte, 0, len(u16)*2)
	for _, c := range u16 {
		buf = append(buf, byte(c), byte(c>>8))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// buildCloudbaseInitUserData は cloud-config を返す。追加ユーザーの作成(対象があれば)と、
// RDP の有効化(runcmd)を含む。"root" は Windows に存在しないため、ユーザーから除外する。
// 公開鍵が指定された場合は、Administrator 用の sshd の公開鍵設定(runcmd)も含む。
// 公開鍵とユーザー("root" 以外)が両方指定された場合は、未実装としてエラーにする。
func buildCloudbaseInitUserData(password, sshKey string, usernames []string) (string, error) {
	type cloudConfigUser struct {
		Name   string `yaml:"name"`
		Groups string `yaml:"groups"`
		Passwd string `yaml:"passwd,omitempty"`
	}
	type cloudConfig struct {
		Users  []cloudConfigUser `yaml:"users,omitempty"`
		RunCmd []string          `yaml:"runcmd"`
	}

	keys := splitSSHKeys(sshKey)
	cfg := cloudConfig{RunCmd: append([]string{}, enableRDPCommands...)}
	for _, username := range normalizeUsernames(usernames) {
		if strings.EqualFold(username, "root") {
			continue
		}
		if len(keys) > 0 {
			return "", fmt.Errorf("ユーザー指定付きの公開鍵設定は Windows では未実装です (spec.auth の user/users と公開鍵は同時に指定できません): %s", username)
		}
		cfg.Users = append(cfg.Users, cloudConfigUser{
			Name:   username,
			Groups: "Administrators",
			Passwd: password,
		})
	}
	if len(keys) > 0 {
		sshdCmds, err := sshdAuthorizedKeysCommands(keys)
		if err != nil {
			return "", err
		}
		cfg.RunCmd = append(cfg.RunCmd, sshdCmds...)
	}
	cfg.RunCmd = append(cfg.RunCmd, runcmdExitCommand)

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to build user_data: %v", err)
	}
	return "#cloud-config\n" + string(out), nil
}

// buildCloudbaseInitNetworkData は、MAC アドレスと静的 IPv4 アドレスを持つ NIC から
// network_data.json を作る。該当する NIC がなければ nil を返す(Windows 既定の DHCP のままにする)。
// NIC は MAC アドレスで照合されるため、NIC の順序には依存しない。
func buildCloudbaseInitNetworkData(nics []api.NetworkInterface) ([]byte, error) {
	var nd cloudbaseInitNetworkData
	for i, nic := range nics {
		if nic.Mac == nil || strings.TrimSpace(*nic.Mac) == "" || nic.Address == nil {
			continue
		}
		address := strings.TrimSpace(*nic.Address)
		if net.ParseIP(address) == nil {
			continue
		}
		netmask := nicNetmask(nic)
		if netmask == "" {
			continue
		}

		linkID := fmt.Sprintf("nic%d", i)
		nd.Links = append(nd.Links, cloudbaseInitLink{ID: linkID, Type: "phy", MacAddress: strings.TrimSpace(*nic.Mac)})
		network := cloudbaseInitNetwork{
			ID:        fmt.Sprintf("net%d", i),
			Link:      linkID,
			Type:      "ipv4",
			IPAddress: address,
			Netmask:   netmask,
			Routes:    nicRoutes(nic),
		}
		if nic.Nameservers != nil && nic.Nameservers.Addresses != nil {
			for _, dns := range *nic.Nameservers.Addresses {
				if dns = strings.TrimSpace(dns); dns != "" {
					network.Services = append(network.Services, cloudbaseInitDNSSvc{Type: "dns", Address: dns})
				}
			}
		}
		nd.Networks = append(nd.Networks, network)
	}
	if len(nd.Networks) == 0 {
		return nil, nil
	}
	data, err := json.MarshalIndent(nd, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to build network_data.json: %v", err)
	}
	return data, nil
}

// nicNetmask は Netmask(ドット表記)を優先し、なければ Netmasklen から IPv4 のネットマスクを求める。
func nicNetmask(nic api.NetworkInterface) string {
	if nic.Netmask != nil && net.ParseIP(strings.TrimSpace(*nic.Netmask)) != nil {
		return strings.TrimSpace(*nic.Netmask)
	}
	if nic.Netmasklen != nil && *nic.Netmasklen >= 0 && *nic.Netmasklen <= 32 {
		return net.IP(net.CIDRMask(*nic.Netmasklen, 32)).String()
	}
	return ""
}

// nicRoutes は Routes(to/via)と IpGateway からルートを求める。デフォルトルートがなければ
// IpGateway を既定ゲートウェイとして使う。CIDR として解釈できない宛先は無視する。
func nicRoutes(nic api.NetworkInterface) []cloudbaseInitRoute {
	var routes []cloudbaseInitRoute
	hasDefault := false
	if nic.Routes != nil {
		for _, r := range *nic.Routes {
			if r.To == nil || r.Via == nil || net.ParseIP(strings.TrimSpace(*r.Via)) == nil {
				continue
			}
			to, via := strings.TrimSpace(*r.To), strings.TrimSpace(*r.Via)
			if to == "default" || to == "0.0.0.0/0" {
				routes = append(routes, cloudbaseInitRoute{Network: "0.0.0.0", Netmask: "0.0.0.0", Gateway: via})
				hasDefault = true
				continue
			}
			_, ipnet, err := net.ParseCIDR(to)
			if err != nil || ipnet.IP.To4() == nil {
				continue
			}
			routes = append(routes, cloudbaseInitRoute{Network: ipnet.IP.String(), Netmask: net.IP(ipnet.Mask).String(), Gateway: via})
		}
	}
	if !hasDefault && nic.IpGateway != nil && net.ParseIP(strings.TrimSpace(*nic.IpGateway)) != nil {
		routes = append(routes, cloudbaseInitRoute{Network: "0.0.0.0", Netmask: "0.0.0.0", Gateway: strings.TrimSpace(*nic.IpGateway)})
	}
	return routes
}

func splitSSHKeys(sshKey string) []string {
	var keys []string
	for _, line := range strings.Split(sshKey, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			keys = append(keys, line)
		}
	}
	return keys
}

// sanitizeWindowsHostname は VM 名を Windows のコンピューター名として使える形式
// (英数字とハイフン、15 文字以内)に変換する。使える文字が残らない場合は空文字列を返す。
func sanitizeWindowsHostname(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > netbiosHostNameMaxLen {
		s = strings.Trim(s[:netbiosHostNameMaxLen], "-")
	}
	return s
}
