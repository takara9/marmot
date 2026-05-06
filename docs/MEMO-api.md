## APIの修正

marmot の APIを記述したYAMLファイルは、apiVesrionとkind が無いため、YAMLを見ただけでは、オブジェクトとの対応が判別できない。
そのため、以下の構造に移行したい。

- apiVersion string
- kind string
- metadata
- spec
- status

## 項目の内容

- apiVersionは、string型で、YAMLファイルでは、固定的に v1をセットする。将来、バージョンを更新する際は、これで対応する処理関数へ分岐を実施する。
- 将来 v2 を開発して移行する際は、"/api/v2"として、処理経路を追加する。
- kind は、image, marmot, network, server, volume など、対応するオブジェクトの種類をセットする。これによって、YAMLファイルが対応するオブジェクトを判別でき、spec 以下の内容の参照先も明確になる
- 
