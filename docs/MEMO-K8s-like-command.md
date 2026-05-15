# K8sに良く似たコマンドにする件


## コマンドの基本的な表記

mactl VERB RESOURCE [OBJECTNAME] [FLAGS] [NAME]

- VERBは、create|apply|get|describe|del から選択できる
- create  OBJECTNAMEで指定したオブジェクトを作成するため、RESORCEのURLアドレスに対してPOSTする。同じオブジェクト名が存在していれば、「既にオブジェクトが存在するので、作成できない」エラーを返す。
- apply   オブジェクトが存在しなければ、RESORCEのURLアドレスに対してPOSTするところは、createと同じ動作をする。既にオブジェクトが存在していれば、PUTして、オブジェクトを更新する。
- get [NAME] NAMEが指定された時は、一致するオブジェクトについてのみ表示する。NAMEが省略された時は、一致するリソースのオブジェクトをリストする。
- describe NAME NAMEは必須で、一致するオブジェクトを見やすく整形されたテキストスタイルで、表示する。
- del NAME NAMEは必須で、一致するオブジェクトを削除する。 

RESOURCEは、以下の４つから選択できる
- server  または srv
- image   または img
- volume  または vol
- network または net

FLAGSは、
- -o text|json|yaml から選択できる。デフォルトは text　
  - textが設定された場合、整形された見やすい形式で表示
  - jsonが指定された場合、インデント無しのjson形式の文字列表示
  - yamlが指定された場合、YAML形式の文字列で表示
- -f filename | URL | stdin
  - VERBが、create または apply の時のオプション
  - ファイル名、または、URL アドレスから、ファイルを読んで、VERBで指定した処理を実行
- -w VERBが、get かつ -o textの時に有効なオプションで、更新が発生した行のみが表示され続ける。Contrl-Cで止められるまで継続する



## マニフェスト

~~~
apiVersion: v1  　# この指定により http://anyhost:8750/api/v1 が決まる
kind: Server      # この指定により、REST-APIのパスが決まる
metadata:
    name: server-100
    comment: 何もしないミニマルな設定
spec:
    cpu: 1
    memory: 1024
    osVariant: ubuntu24.04
    auth:
~~~


Kind             Path       apiVersionとKindの結果
---------------  ---------  --------------------------
Server           /server    /api/v1/server
Volume           /volume    /api/v1/volume
VirtualNetwork   /network   /api/v1/network
Image            /image     /api/v1/image

