## 結論

RFC 6762（Multicast DNS、2013年）には「.local を unicast DNS に使うのは NOT RECOMMENDED」という一文があるわけではありません。実際には、**Section 3 の MUST 規定**と **Section 22（Special-Use 登録）** によって .local を mDNS 専用と定めています。そのうえで **Appendix G** が、.local を unicast で流用すると問題が起きると明記しています。この三つを合わせて「非推奨」と言われています。

## 該当箇所の整理

| 箇所 | 種別 | 内容（要旨） |
|---|---|---|
| Section 3 "Multicast DNS Names" | 規範（MUST） | 名前が `.local.` で終わる DNS クエリは、mDNS マルチキャストアドレス（224.0.0.251 / FF02::FB）へ送らなければならない |
| Section 3（続き） | 規範の補足 | その `.local` が、ユーザーが FQDN を入力したものか、search list（手動設定でも DHCP 由来でも）で付加されたものかは関係ない |
| Section 22 / 22.1 | IANA 登録 | `.local.` と link-local 逆引きゾーンを RFC 6761 の Special-Use Domain Names に登録している。名前解決 API やライブラリは、これらの名前を unicast のキャッシュ DNS サーバーへ送るべきでない（SHOULD NOT）としている |
| Appendix G "Private DNS Namespaces" | 非規範（解説） | 未登録 TLD の使用自体を推奨しない。そのうえで、私設網で `.local` を流用すると問題が出ると指摘し、過去に使われた例として `.intranet` `.internal` `.private` `.corp` `.home` `.lan` を挙げている |

要するに、仕様に準拠した実装は `.local` を unicast DNS サーバーに問い合わせないことになっています。そのため、DNS サーバー側で `labo.local` ゾーンを正しく運用していても、クライアントが問い合わせに来ないという構造的な問題が起きます。

## 実環境で起きること

| 環境 | 挙動 |
|---|---|
| macOS / iOS（Bonjour） | `.local` はまず mDNS で解決される。unicast へのフォールバックは、条件付きで遅延が出たり失敗したりする |
| Linux + nss-mdns（Avahi） | `hosts: files mdns4_minimal [NOTFOUND=return] dns` の設定では、mDNS で見つからなかった時点で打ち切られ、unicast DNS に到達しない |
| Linux + systemd-resolved | `.local` は mDNS/LLMNR 側に振り分けられる。unicast に流すにはリンクごとのドメイン設定（例：`~local`）などの明示的な設定が必要 |
| Windows（AD） | かつては `corp.local` 型の AD ドメイン名が広く使われたが、Microsoft も現在は推奨していない |
| 公開 CA | 内部名への証明書発行は CA/B Forum の規定で禁止されている。私設 CA（step-ca など）なら発行自体は可能 |

ここから、クライアント OS の種類によって「名前が引けたり引けなかったりする」「数秒の遅延が出る」といった、再現性の低いトラブルが典型的に起きます。

## 推奨される代替

| 名前 | 根拠 | 用途 |
|---|---|---|
| `home.arpa` | RFC 8375（Special-Use 登録済み） | 家庭・ラボ網向けに標準化された名前 |
| `.internal` | ICANN が2024年に私的利用向けとして予約 | 社内やラボの私設名前空間 |
| 自分が所有する公開ドメインのサブドメイン（例：`lab.example.com`） | 通常の DNS | 最も確実。将来的に公開 CA の DNS-01 認証も使える |

ラボの `ca.labo.local` も同じ問題の対象になります。step-ca 自体は `.local` 名でも証明書を発行できますが、ACME クライアントや各ノードの名前解決が mDNS 側に吸われる可能性があります。新しく作り直す機会があれば、`labo.internal` や `labo.home.arpa` への移行を検討する価値があります。