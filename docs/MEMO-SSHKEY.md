# SSHキーをVMのOSに組み込む機能の追加

公開鍵のURL、または、ファイル名から、公開鍵を仮想マシンのOSに設定する。


## 仕様

1. mactl server create -f VM設定ファイル名　を指定して、仮想マシンを起動する。
1. VM設定ファイルに指定されたURLからダウンロードして、OSのホームディレクトリに設定する。
1. オプションでファイル名を指定して、root のホームディレクトリに設定する
1. HOME/.ssh を作成 モードは0700
1. HOME/.ssh/authorized_keysを作成、ダウンロードまたは指定されたファイルから読み込んだ公開鍵をセットする

## 実装方法

追加が必要な情報

- 公開鍵URL　　　GitHubなどからダウンロードする際に利用
- 公開鍵内容     ファイル渡しで、公開鍵を指定する場合に利用
- ユーザー    　 鍵を格納するユーザーとディレクトリ /home/ユーザー名r で作成、その下に /home/user/.ssh/authorized_keysとして配置

### APIの追加部分


```
    ServerSpec:
      type: object
      properties:
        cpu:
          type: integer
          format: int
        memory:
          type: integer
          format: int
        NetworkInterface:
          type: array
          maxItems: 10
          items:
            $ref: "#/components/schemas/NetworkInterface"
        Storage:
          type: array
          maxItems: 10
          items:
            $ref: "#/components/schemas/Volume"
        osLv:
          type: string
        osVg:
          type: string
        osVariant:
          type: string
        bootVolume:
          $ref: "#/components/schemas/Volume"
        auth:
          $ref: "#/components/schemas/Auth"
```

```
    Auth:
      type: object
      properties:
        url:
          type: string
        publicKey:
          type: string
        username:
          type: string
```