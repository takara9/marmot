# mactl認証と認可機能の実装

## 要旨
本文書は、mactl コマンドに、ユーザー認証と認可機能を実装するためのメモである。
この文章のスコープはmactl コマンドであり、ゲストVMの認証はスコープ外である。

## 認証
- mactl コマンドは、ユーザーIDとパスワードで認証を通過することで、mactl は marmotd 連携することができる。
- mactl は、ユーザーIDとパスワードを marmotd に送信して、marmotd に保存された bcrypt ハッシュ化され保存されたデータと照合する。
- mactl コマンドと marmotd 間の連携は、インターネット越しで通信するため、TLSで暗号化されたTCP/IP通信でコミュニケーションを取る。（現状は未実装で、今後実装予定） 
- 初期ユーザーIDは、管理者ユーザー `admin`のみとして、ロールは`Administrator`とする。
- 管理者パスワードは、`passw0rd`をbcryptでハッシュされたパスワードをユーザーID`admin`のパスワードにセットする。
- 初回ログイン時に変更を義務づける。ホームラボや顔が判る範囲の小規模運用を想定しているため、現時点ではこれ以上の対策は実施しない。
- `mactl login USER-ID`で、パスワードのプロンプトでインプットしたパスワードで、認証に成功したら、操作可能となる。
- ユーザーとロールの情報は、etcdに保存して利用する。
- APIキーは、ベアラトークンとして、HTTPヘッダーにセットして使用する。
- APIキーは、作成者のロールの権限の範囲で処理ができる。
- APIキーが存在しない場合、期限が切れている場合は、認証エラーを返し、要求を受け付けない。

## ユーザー管理コマンド

パスワードの条件は、英数字のみ、最低8文字とする。パスワードの設定判定はモジュール化して、文字数、記号、大文字小文字混在などは後から追加が容易な構造にする。初期の実装では、ユーザーはフラットな構造として、GroupやOrgでグループ別けしない。

グループは、`default` １種だけとして、全ユーザーはすべて、`default`グループに属する。

### ユーザー用コマンド

- `mactl login USER-ID` ログイン、他ユーザーでログインしている時は、ログインが成功すると既存セッションはログアウトする
- `mactl whoami` 自分のユーザーIDとロールを表示、`mactl role` のエリアス
- `mactl role` 自身に付与されたロールのリスト表示
- `mactl role list` 組み込みロール一覧の表示
- `mactl logout`
- `mactl passwd` ユーザーのパスワード変更用
- `mactl user generate-apikey --comment TEXT` APIキーを生成する。コメントは任意
- `mactl user list-apikey` 自分が発行したID,コメントをリストする 
- `mactl user delete-apikey API-ID` IDでAPIキーを削除して無効化する 

### 管理者用コマンド
管理者が設定したIDとパスワードは、秘匿性の高い手段を使用して配布されなければならない。

- `mactl user add-role USER-ID ROLE-NAME` ユーザーにロールを追加
- `mactl user del-role USER-ID ROLE-NAME` ユーザーのロールの削除
- `mactl user list-role USER-ID` ユーザーのロールをリスト
- `mactl user add USER-ID --role ROLE-NAME --passwd PASSWORD` USER-IDをロール付き、初期パスワード付きで作成。オプションは必須とする。
- `mactl user delete USER-ID` USER-IDの削除
- `mactl user set-passwd USER-ID --passwd PASSWORD` USER-IDのパスワード再セット用
- `mactl user lock USER-ID` ユーザーの使用を停止する。ユーザーのAPIキーも同様に無効化される。
- `mactl user list` ユーザー一覧の表示、表示列: USER-ID / ENABLED / ROLE / AGE

### ユーザーがパスワードをロスした時の処理
ユーザーがパスワードを忘れた場合、管理者はユーザーの申告を受けてパスワードを再セットできる。
管理者は、本人確認は、Zoom等で対話するなどして、本人を確認しなければならない。

## 認可
- ユーザーは、割当られたロールで、コマンドを実行する権限が与えられる RBAC方式を採用する。
- `/authz/check` は、`userId` 未指定時は認証済みの自ユーザーを照会対象とする。
- `/authz/check` で `userId` を指定する場合、照会可能なのは「自ユーザー」または `Administrator` ロールのみとする。
- `Administrator` 以外が他ユーザーを指定して照会した場合は、`403 forbidden` を返す。
- ロールには、以下の種類と権限がある。
    - Administrator
        - Server: 作成, 参照, 更新, 削除
        - Cluster: 作成, 参照, 更新, 削除
        - Volume: 作成, 参照, 更新, 削除
        - Network: 作成, 参照, 更新, 削除
        - ServerGateway: 作成, 参照, 更新, 削除
        - VpnGateway: 作成, 参照, 更新, 削除
        - NetworkLoadBalancer: 作成, 参照, 更新, 削除
        - ApplicationLoadBalancer: 作成, 参照, 更新, 削除
        - User: 作成, 参照, 更新, 削除

    - Network-Administrator
        - Server: 参照
        - Cluster: 参照
        - Volume: 参照
        - Network: 作成, 参照, 更新, 削除
        - ServerGateway: 参照
        - VpnGateway: 作成, 参照, 更新, 削除
        - NetworkLoadBalancer: 作成, 参照, 更新, 削除
        - ApplicationLoadBalancer: 作成, 参照, 更新, 削除
        - User: 参照

    - Compute-Operator
        - Server: 作成, 参照, 更新, 削除
        - Cluster: 参照
        - Volume: 作成, 参照, 更新
        - Network: 参照
        - ServerGateway: 作成, 参照, 更新, 削除
        - VpnGateway: 参照
        - NetworkLoadBalancer: 参照
        - ApplicationLoadBalancer: 参照
        - User: 参照

    - Viewer
        - Server: 参照
        - Cluster: 参照
        - Volume: 参照
        - Network: 参照
        - ServerGateway: 参照
        - VpnGateway: 参照
        - NetworkLoadBalancer: 参照
        - ApplicationLoadBalancer: 参照
        - User: 参照

## 監査用記録

初期バージョンでは、実装しないが、監査ログは、PostgreSQLデータベースに保存する

保存用テーブル

```sql
CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMP NOT NULL DEFAULT now(),
    user_id UUID NOT NULL,
    organization_id UUID NOT NULL,
    action VARCHAR(50) NOT NULL,
    resource VARCHAR(100) NOT NULL,
    resource_id VARCHAR(100),
    result VARCHAR(20) NOT NULL,        -- success / denied
    trace_id VARCHAR(64),
    request_ip VARCHAR(45),
    detail JSONB
);

-- UPDATE/DELETEを禁止するルールやトリガーで改ざん防止
REVOKE UPDATE, DELETE ON audit_logs FROM app_user;
GRANT INSERT, SELECT ON audit_logs TO app_user;
```


