# イメージ管理

## 対象OS Ubuntu, Rockey, VyOS

以下に挙げる３種類のソフトウェアをmarmotクラウド上の仮想マシンとして利用できるようするする。

- Ubuntu Cloud Image: https://cloud-images.ubuntu.com/
- Rockey Cloud Image: https://dl.rockylinux.org/pub/rocky/
- VyOS router image: https://docs.vyos.io/en/latest/installation/virtual/libvirt.html

## イメージの内容

イメージには、OS起動イメージと仮想マシンの定義がある。
- OS起動イメージ   imgファイルで内容は qcow2 または、 lv 論理ボリュームをddでファイル化したもの
- VM稼働マシン     libvirt の xml ファイル

マシンイメージは、上記２つの情報をパッケージにしたものとなる。


## 実行中の仮想マシンからイメージを生成する方法

以下のステップで、イメージを生成する。

1. 実行中のマシンを一時停止する
1. イメージにIDを付与する
1. /var/lib/marmot/images に、idのフォルダーを作成する
1. 仮想マシンのmarmotのデータをJSON形式で、作成したフォルダーに保存する
1. OSイメージを、作成したフォルダーにコピーする
1. libvirt で dumpxml を実行して、xmlファイルを生成して、上記で作成したフォルダーに保存する
1. 実行中のマシンの稼働を再開する
1. イメージの情報をデータベースに保存する
1. オブジェクトストレージに保存する
1. /var/lib/marmot/images のワークエリアを削除する

## イメージから仮想マシンの生成方法

以下のステップで、イメージから仮想マシンを生成する。

1. 仮想マシンにIDを付与する
1. JSONオブジェクトから、利用するイメージのKEYを取得して、KEYで指定されたOSイメージから、新たなボリュームを生成する
1. JSONのオブジェクトから、接続先の仮想ネットワークを決定する
1. 仮想ネットワークに付随するIPネットワークからNICにアドレスを付与する。
1. JSONのオブジェクトで、指定されたテンプレートイメージから、ボリュームをコピーまたはスナップショットを作ってボリュームを生成する
1. 生成したボリュームをマウントして、OSの設定ファイルを作成する。
1. OS設定が完了したら、アンマウントする。
1. 仮想マシンのオブジェクトデータから、libvirtのxmlを生成する
1. 仮想マシンを定義して、実行を開始する


## イメージの作成コマンド #1 稼働中のマシンから
以下のコマンドで、イメージの作成が開始される。
これは、OSのバックアップを兼ねる

```console
$ mactl server makeimage [server id] [image name]
イメージの作成が開始されました。
```

 作成結果は、以下のコマンドで確認できる

 ```console
 $ mactl image list
 ```

 ```console
 $ mactl image detail [id]
 ```


## イメージの作成コマンド #2 
インターネット上のクラウドイメージのURLから、データをダウンロードして、テンプレートとになるイメージを作成する

```console
$ mactl image createtemp --name ubuntu22.04 https://cloud-images.ubuntu.com/releases/jammy/release-20260218/ubuntu-22.04-server-cloudimg-amd64.img 
```


## イメージを使った仮想マシンの生成

 ```yaml
name: test-server-40
cpu: 2
memory: 2048
volume_type: lvm
image: image_keyword  # こちらは廃止の方向で
boot_volume:
  type: lvm         # または qcow2
  image: image_name # あらたに追加
network:
  - name: "host-bridge"
comment: "This is a test server configuration"
```

起動コマンドは以下とする

```console
$ mactl server create -f test-server-40.yaml
```

## イメージの削除

以下のコマンドで、データベースと/var/lib/marmot/images/id から削除する
オブジェクトストレージと連携すれば、そちらの削除も実行する

```console
$ mactl image delete [id]
```

## 作成するコマンドの再整理

- mactl image list
- mactl image detail [id]
- mactl image createtemp --name [image name] URL
- mactl image delete [id]
- mactl server makeimage [server id] [image name]

## データベースの追加

イメージを管理するための構造体を追加する

type Image struct {
	Id       string      `json:"id"`
	Metadata *Metadata   `json:"Metadata,omitempty"`
	Spec     *ImageSpec  `json:"Spec,omitempty"`
	Status   *Status     `json:"Status,omitempty"`
}

Specは、VolSpecと共用するか、それとも ImageSpecを新たに作るか？検討が必要

type VolSpec struct {
	Kind          *string `json:"kind,omitempty"`             // OS or DATAの区別
	Type          *string `json:"type,omitempty"`             // LV or QCOW2の区別
	VolumeGroup   *string `json:"volumeGroup,omitempty"`      // 必要
	LogicalVolume *string `json:"logicalVolume,omitempty"`    // 必要
    OsVariant     *string `json:"osVariant,omitempty"`        // 未使用
	Path          *string `json:"path,omitempty"`             // QCOW2の時のパス
	Size          *int    `json:"size,omitempty"`             // イメージのサイズ
	Persistent    *bool   `json:"persistent,omitempty"`       // 未使用
}

libvirt XML形式のデータは、どこに保持するか？ /var/lib/marmot/image/ID の下に固定ファイル名で保持
イメージのディレクトリは、どこに保持するか？ /var/lib/marmot/image/XXXに固定ファイル名で保持
オプジェクトストレージのURLは、どうするか？  必須にはしたくない。オプションが良い

## イメージコントローラー

- ジョブが必要になるので、どうするか？　OpenAPIで作り直しが必要
- 現在、コントローラーが複数のパッケージに別れているが、意味がないので、一つのパッケージにまとめたい。先にそれを実施するのが良いか？