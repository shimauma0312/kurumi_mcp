# ConoHa VPSへの配置

Ubuntu 24.04のVPSで、Secure MCP Tunnelからwalnut-mcpをstdioプロセスとして起動します。

## 配置構成

```text
/srv/discord-bots/walnut-mcp/walnut-mcp # Linux実行ファイル
/srv/discord-bots/walnut-mcp/.env       # 秘密情報を含む環境変数
```

`tunnel-client`がwalnut-mcpをstdioで起動します。VPSのファイアウォールへMCP用ポートを追加せず、Nginxも使用しません。

## 環境変数

VPSの`/srv/discord-bots/walnut-mcp/.env`では、stdioトランスポートを指定します。

```dotenv
MCP_TRANSPORT=stdio
MCP_ADDR=
MCP_BEARER_TOKEN=
```

その他の設定は、ルートの`.env.example`を参照してください。環境ファイルの所有者は`walnut-mcp`、権限は`0600`にします。

## 動作確認

`tunnel-client doctor`で、トンネルとstdioプロセスをまとめて検証します。実際のプロファイル名はトンネル作成時に決定します。

## 更新

新しいLinux実行ファイルへ置き換えた後、`tunnel-client`のsystemdサービスを再起動します。
