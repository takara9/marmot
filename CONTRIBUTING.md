# Marmotプロジェクトへの貢献

Marmotプロジェクトへの貢献をご検討いただきありがとうございます！
バグ報告、機能リクエスト、ドキュメントの改善、コードなど、あらゆる貢献を歓迎いたします。

# 貢献方法

## Issues

次のIssueを受け付けています。

- 書籍や内容に対する質問 => [こちらから質問できます](https://github.com/takara9/marmot/issues/new?template=question.md)
- 内容のエラーや問題の報告 => [こちらからバグ報告できます](https://github.com/asciidwango/js-primer/issues/new?template=bug_report.md)
- 解説の改善を提案 => [こちらから提案できます](https://github.com/asciidwango/js-primer/issues/new?template=feature_request.md)
- 新しいトピックなどの提案 => [こちらから提案できます](https://github.com/asciidwango/js-primer/issues/new?template=feature_request.md)

上記以外に[その他のIssue](https://github.com/asciidwango/js-primer/issues/new?template=other.md)も歓迎しています。


## Pull Request

Pull Requestはいつでも歓迎しています。

**受け入れるPull Request**

次の種類のPull Requestを受け付けています。
基本的なPull Request（特に細かいもの）は、Issueを立てずにPull Requestを送ってもらって問題ありません。

「このような修正/改善はどうでしょう？」という疑問がある場合は、Issueを立てて相談してください。

- 誤字の修正
- サンプルコードやスペルの修正
- 別の説明方法の提案や修正
- 文章をわかりやすくするように改善
- ウェブサイトの改善
- テストの改善

:memo: **Note:** Pull Requestを受け入れるとあなたの貢献が[Contributorsリスト](https://github.com/takara9/marmot/graphs/contributors)に追加されます。
また、Pull Requestを送った内容はこのドキュメントのライセンス（[MIT](./LICENSE-MIT)と[CC BY 4.0](./LICENSE-CC-BY)）が適用されます。
これは、あなたの貢献がこの書籍への努力的な寄付となることを意味しています。

**受け入れていないPull Request**

- [CODE OF CONDUCT](./.github/CODE_OF_CONDUCT.md)に反する内容を含むもの

## 修正の送り方

基本的に次の手順を試してください。ドキュメントの修正などは、テストの必要はありません。

1. Branchを作る: `git checkout -b my-new-feature`
2. コードの編集: お好みの統合開発環境やエディタで、コードの追記、修正などを実施
3. テストする: `make test`, 修正箇所にあるMakefileを使ってテストができます。
4. 変更をコミットする: `git commit -am 'feat: add some feature'`
5. Pushする: `git push origin my-new-feature`
6. Pull Requestを送る :D

**注意点**
- テストを実行するためには、ご自身の開発環境のOS環境でパッケージの追加、設定、Docker実行環境などが必須です。専用のPC、または、linux nested virtualization の環境を、ご自身で準備してください。
- marmotのコードをテストする際は、LVM, Virtなどを操作するため、root権限でテストしなければなりません。

## ディレクトリ構造
marmotのディレクトリの構造について解説します。

```
marmot root
├── api
├── cmd
├── cmd
│   ├── hv-admin
│   ├── install.sh
│   ├── mactl
│   ├── marmotd
│   └── migtool
├── docs
├── Makefile
├── pkg
│   ├── client
│   ├── config
│   ├── db
│   ├── dns
│   ├── lvm
│   ├── marmotd
│   ├── types
│   ├── util
│   └── virt
├── README.md
├── sample-servers
└── tools
```

- api: OpenApiの定義や構造体のコードが入っています。marmotのデーモンを操作するためのREST-APIの仕様は、ここに書かれています。
- cmd: クライアントのコマンド、サーバーのコマンドなど、実行形式にするための main関数を持つコマンドが集めています。
    - hv-admin: インストール後のセットアップに必要なコマンド
    - install.sh: インストールシェル
    - mactl: クライアントコマンド
    - marmotd: marmotデーモンのコマンド
    - migtool: バージョン更新時のデータベースの移行ツールなど
- docs: HOW-TOなど、ドキュメント、ノウハウ、メモなどを
- pkg: この下には、目的別のパッケージを集めています。
    - client: marmot デーモンにアクセスするための Go 関数群
    - config: 設定のためのファイルを読み込み構造体に格納するための関数群
    - db: データベース (etcd) をアクセスするための関数群
    - dns: CoreDNS を操作する関数群(実験的)
    - lvm: Linux LVMを操作するための関数群
    - marmotd: marmot デーモンの機能を実装する関数群
    - types: api以外で必要な内部的な構造体群
    - util: 便利関数群
    - virt: 様々な仮想化環境を統一的なAPIで操作・管理するためのGoの関数群
- sample-servers: marmotで構築するサーバーのサンプル、Ansibleプレイブックなど
- tools: その他、一時的に使用する便利ツールなど

## コミットメッセージ規約

以下のような形で、記述することを推奨します。

- 1行目に概要
- 2行目は空行
- 3行目から本文

最後に関連するIssue(任意)を書きます。
`fix #<issue番号>` のように書くことで、PRをマージした時に自動的にIssueを閉じることができます。

- [Linking a pull request to an issue - GitHub Docs](https://docs.github.com/en/issues/tracking-your-work-with-issues/linking-a-pull-request-to-an-issue)

```
feat(ngInclude): add template url parameter to events

The `src` (i.e. the url of the template to load) is now provided to the
`$includeContentRequested`, `$includeContentLoaded` and `$includeContentError`
events.

Closes #8453
Closes #8454
```


```
                         scope        commit title

        commit type       /                /      
                \        |                |
                 feat(ngInclude): add template url parameter to events

        body ->  The 'src` (i.e. the url of the template to load) is now provided to the
                 `$includeContentRequested`, `$includeContentLoaded` and `$includeContentError`
                 events.

 referenced  ->  Closes #8453
 issues          Closes #8454
```

`commit type` としては次のようなものがあります。

- feat
    - 新しい機能、章、節の追加など
    - 更新履歴に載るような新しいページを追加
- fix
    - 意味が変わる修正
    - 更新履歴に載るような修正
- docs
    - 基本的には使わない
    - README.mdやCONTRIBUTING.mdや本体のプロジェクト全体のドキュメントについて
- refactor
    - 意味が変わらない修正
    - 更新履歴に載らないような修正
- style
    - スペースやインデントの調整
    - Lintエラーの修正など
- perf
    - パフォーマンス改善
- test
    - テストに関して
- chore
    - その他
    - typoの修正など


`commit type`は、迷ったらとりあえず`chore`と書きます。
`scope`も省略して問題ないので以下のような形でも問題ありません。

```
chore: コミットメッセージ
```

