今回の提案は、auto ポリシー時のピア解決を以下に変更するものです。

1. ハブノードは head ノードを優先
2. スポークノードはハブにのみトンネルを張る
3. ハブノードは全スポークにトンネルを張る
4. これまでの host-status full-mesh フォールバックは廃止（ループ再発防止）

変更箇所
- ピア解決ロジックの hub-spoke 化: network-controller.go
- ハブ選出ヘルパー追加: network-controller.go
- 役割判定ヘルパー追加: network-controller.go
- 決定論的ハブ選出追加: network-controller.go
- hub-spoke の peer IP 組み立て追加: network-controller.go
- ユニットテスト追加: network_topology_test.go
- API コメント更新（auto=hub-spoke）: marmot-api-v1.go
- OpenAPI 説明更新（auto=hub-spoke）: marmot-api-v1.yaml


確認結果
- go test ./pkg/controller は成功
- go test ./pkg/marmotd -run TestNonExistent -count=1（コンパイル確認）は成功

補足
- 現在 manual は peer 解決を返さない動作のため、将来的に「明示 peer 指定」を使いたい場合は、manual 用の peer 入力項目を API に追加するのが次の拡張ポイントです。
- Issue のエラーメッセージにある ofport=-1 は OVS ポート状態に依存する場合があり、今回の変更は主にループ抑止の対策です。必要であれば、次段で ofport=-1 を無視/再試行する防御ロジックも提案できます。

現状実装では、3ノードで1ノード停止時の挙動は「停止したノードがどの役割か」で変わります。

1. 共通動作
- 生存ノードのコントローラーは定期ループで再収束します（ACTIVE時にVXLAN再同期）。
- ピア候補ノード集合はネットワークレコードから取り、実際に使うIPはHostStatusから引きます。
- 停止ノードのHostStatusが消えていれば、そのノードはpeer計算から外れ、不要トンネルはpruneで削除されます。  
参照: network-controller.go, network-controller.go, ovs.go

2. 停止ノードがスポーク（非ハブ）の場合
- ハブは生存スポークだけをpeerとして持つように再計算されます。
- 残りスポークは引き続きハブにのみ接続します。
- つまり、停止スポークへのトンネルは消え、残り2ノード間（ハブ-スポーク）は継続します。  
参照: network-controller.go

3. 停止ノードがハブ（headNode）の場合
- スポークは「ハブにのみ接続する」設計なので、ハブIPが取れないとpeerが空になります。
- その結果、スポーク側トンネルは消え、オーバーレイ通信は止まります（ハブ復帰待ち）。  
参照: network-controller.go, ovs.go

4. split-horizonまわり
- 今回の防御で、ofportが未準備（-1/0）でもERRORにせずスキップするため、停止直後の揺らぎでNETWORK_ERROR化しにくくなっています。  
参照: ovs.go, ovs.go

要点
- ループ抑止は達成されています。
- ただし可用性はハブ依存です。ハブ停止時は設計上、通信断になります。  
必要なら次段で、ハブ障害時フェイルオーバー（新ハブ再選出）案まで具体化できます。




実現方針
1. 検知: 現ハブが非アクティブかを HostStatus の LastUpdated で判定
2. 選出: 生存ノードから新ハブを決定的に1台選ぶ
3. 切替: ネットワーク群のラベルを一括更新して新ハブに寄せる
4. 収束: 既存の VXLAN 再同期ループでトンネルを自動再構成

既存資産との整合
- ネットワーク同期ラベルは既にある: db-virtual_network.go
- ACTIVE ループで定期再収束する: network-controller.go
- hub 候補参照は labels と head 判定を既に利用中: network-controller.go
- HostStatus の更新時刻がある: host.go
- アクティブ閾値 30 秒が既にある: scheduler.go

具体アルゴリズム
1. 対象ネットワーク抽出
- overlayMode=vxlan かつ peerPolicy=auto のみ対象
- 同名ネットワーク群を1クラスタとして処理

2. 現ハブ判定
- 第一候補: labels.headNodeName
- 第二候補: syncRole=head のエントリ
- それでも空なら現状の deterministic 選出にフォールバック

3. 障害検知
- 現ハブが active hosts に含まれなければ障害と判定
- active hosts は LastUpdated > now - ActiveHostThreshold

4. 新ハブ選出
- 候補は「同名ネットワーク群に参加中 かつ active host」
- 決定規則は固定:
- 第1キー hostId 昇順
- 第2キー nodeName 昇順
- hostId 取得不能ノードは末尾扱い

5. 切替トランザクション
- etcd ロックを取得: 例 /lock/virtualnetwork/failover/<networkName>
- ロック取得後に再読込して二重選出を防止
- 新ハブ1件を head に更新:
- labels.syncRole=head
- labels.headNetworkId=""
- labels.headNodeName=<newHub>
- metadata.nodeName=<newHub>
- 残りを follower に更新:
- labels.syncRole=follower
- labels.headNetworkId=<newHeadNetworkId>
- labels.headNodeName=<newHub>
- metadata.nodeName は各ノード値を維持
- status.message に failover 情報を記録
- 例: sync:hub-failover old=marmot1 new=marmot2

6. 切替後の動作
- 新ハブは全スポークへ接続
- スポークは新ハブへ1本のみ接続
- 旧ハブ復帰後は follower として再参加（自動プリエンプトしない）

挿入ポイント
1. ACTIVE 処理の冒頭でフェイルオーバー判定
- network-controller.go

2. 追加する主関数（同ファイル）
- ensureVxlanHubFailover(vnet)
- collectActiveHostSet(statuses)
- selectHubCandidate(nodes, statuses)
- applyHubFailover(networkName, newHub)

3. DB 補助の追加（任意だが推奨）
- ネットワーク群を同名で更新する補助関数
- db-virtual_network.go 付近に追加

最小変更での実装順
1. controller 側のみで実装
- まずは既存 UpdateVirtualNetworkById を使って切替
- 期待効果を先に出す

2. その後 DB 側に専用ヘルパー化
- 再読込・ロック・更新を集約して保守性を上げる

3. テスト追加
- 新規: network_hub_failover_test
- ケース:
- 3ノードでハブ停止 -> 新ハブ選出される
- 旧ハブ復帰 -> follower で残る
- active host が1台のみ -> hub はその1台
- active host が0台 -> 何もしない

障害時の収束イメージ
- 現在ハブ A, スポーク B/C
- A 停止
- 30秒超で A 非アクティブ判定
- 新ハブ B を選出
- labels 一括更新
- B: peer=[C], C: peer=[B] に再収束
- 通信復旧

注意点
1. 切替は必ずロック下で再読込して実行（分散同時切替防止）
2. フェイルオーバー後の即時戻しはしない（フラップ抑止）
3. 新ハブ不在時は無理に切替せず現状維持

この方針なら、既存の hub-spoke と ofport 防御を活かしつつ、高可用化を段階導入できます。  
必要なら次の返答で、この設計をそのままコードに落として実装まで進めます。



実装フェーズに入ります。まずは network controller にハブ障害検知と再選出を組み込むため、既存ヘルパーと依存関数を確認して、最小差分で動く形に落とし込みます。

Read [](file:///home/ubuntu/marmot/pkg/controller/network-controller.go#1-1), lines 1 to 260

Read [](file:///home/ubuntu/marmot/pkg/controller/network-controller.go#260-260), lines 260 to 560

Read [](file:///home/ubuntu/marmot/pkg/controller/network-controller.go#560-560), lines 560 to 705

Read [](file:///home/ubuntu/marmot/pkg/controller/server-controller.go#1-1), lines 1 to 120

Read [](file:///home/ubuntu/marmot/pkg/marmotd/scheduler.go#1-1), lines 1 to 120

フェイルオーバー実装の差し込み位置は確認できました。これから controller に「アクティブホスト判定・新ハブ選出・同名ネットワーク群のラベル更新」を追加し、ACTIVE処理へ組み込みます。型名の不整合を避けるため、関連シンボルを最終確認してから一気にパッチを当てます。

Searched for regex `GetHeadNodeName|GetHeadNetworkNodeName|SetNetworkSyncLabels|MakeFollowerVirtualNetworkEntry` (`**/pkg/**`), 12 results

Searched for regex `VirtualNetworkID\(|SetVirtualNetworkID\(|Metadata\.Id` (`**/api/**`), 14 results

Read [](file:///home/ubuntu/marmot/pkg/db/db-virtual_network.go#460-460), lines 460 to 551

実装方針を確定しました。まずネットワークコントローラーに「リーダーのみ実行するハブ障害フェイルオーバー処理」を追加し、既存 ACTIVE ループに組み込みます。その後、選出ロジックのユニットテストを追加して検証します。

Generating patch (284 lines) in [](file:///home/ubuntu/marmot/pkg/controller/network-controller.go)


```

```


Generating patch (69 lines) in [](file:///home/ubuntu/marmot/pkg/controller/network_failover_test.go)


```

```
フェイルオーバー本体とテストを追加したので、いまからビルド・テストで不整合を潰します。失敗が出たらその場で修正して通すところまで進めます。

Ran terminal command:  go test ./pkg/controller

Ran terminal command:  go test ./pkg/networkfabric

実装はテストまで通りました。最後に、変更点を追いやすいよう行番号を整理して、フェイルオーバーの実動作を具体的に共有します。

Ran terminal command:  nl -ba pkg/controller/network-controller.go | sed -n '70,260p'

Ran terminal command:  nl -ba pkg/controller/network-controller.go | sed -n '700,980p'

Ran terminal command:  nl -ba pkg/controller/network_failover_test.go

Ran terminal command:  git diff -- pkg/controller/network-controller.go pkg/controller/network_failover_test.go | sed -n '1,240p'

Read [](file:///home/ubuntu/.config/Code/User/workspaceStorage/339a850f45530f1d2996ff4fcdb248d3/GitHub.copilot-chat/chat-session-resources/f062b58f-7d1d-4b86-8d29-556a8f95ac50/call_Nv4XSGgs59viHcBDEj4ZO7v7__vscode-1779746764987/content.txt#1-1), lines 1 to 220

Read [](file:///home/ubuntu/.config/Code/User/workspaceStorage/339a850f45530f1d2996ff4fcdb248d3/GitHub.copilot-chat/chat-session-resources/f062b58f-7d1d-4b86-8d29-556a8f95ac50/call_atckoY2Q6wnNnkdzB7zBuPjI__vscode-1779746764988/content.txt)

Read [](file:///home/ubuntu/.config/Code/User/workspaceStorage/339a850f45530f1d2996ff4fcdb248d3/GitHub.copilot-chat/chat-session-resources/f062b58f-7d1d-4b86-8d29-556a8f95ac50/call_X8Pb945wjF7sV6XoAdZ256LH__vscode-1779746764990/content.txt#1-1), lines 1 to 260

Created memory file [](file:///home/ubuntu/.config/Code/User/workspaceStorage/339a850f45530f1d2996ff4fcdb248d3/GitHub.copilot-chat/memory-tool/memories/repo/vxlan-hub-failover-leader-gated.md)

最終仕上げとして、ネットワーク専用ヘルパーを使うように1点だけ整えておきます。これで可読性を上げてから完了報告します。

Generating patch (12 lines) in [](file:///home/ubuntu/marmot/pkg/controller/network-controller.go)


ハブ障害時フェイルオーバーを実装しました。  
既存の hub-spoke 構成に対して、ハブ停止時に新ハブを自動再選出し、ラベルを書き換えて再収束する動きです。

実装内容
1. リーダー限定でフェイルオーバー実行
- NETWORK ループ内で HostStatus を取得し、スケジューラーリーダーのみが failover を実行するようにしました。
- 参照: network-controller.go

2. VXLAN auto ネットワーク群の failover 再調停を追加
- 同名ネットワークをグルーピングし、現ハブが非アクティブなら新ハブを選出します。
- 参照: network-controller.go

3. ハブ切替処理を実装
- head/follower ラベルの再設定
- headNetworkId, headNodeName の更新
- promoted ノードに head を付与
- 旧 head の IpNetworkId を必要時に引き継ぎ
- 非 head は IpNetworkId を nil 化
- 参照: network-controller.go

4. アクティブホスト判定と選出ロジックを追加
- LastUpdated と ActiveHostThreshold で active 判定
- hostId 優先、同点は nodeName 昇順で決定
- 参照: network-controller.go
- 参照: network-controller.go
- 参照: network-controller.go

5. テスト追加
- active 判定テスト
- hostId 優先選出テスト
- hostId 不正時の nodeName フォールバックテスト
- 参照: network_failover_test.go

動作イメージ
1. 3ノードで hub=A, spoke=B/C
2. A が停止して HostStatus が閾値超過
3. リーダーノードが B/C から新ハブを決定
4. ネットワークラベルを新ハブ基準に更新
5. 既存の ensure/prune ループで VXLAN が再収束

実行確認
1. go test ./pkg/controller 成功
2. go test ./pkg/networkfabric 成功

