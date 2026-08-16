# サーバーへのデプロイ

walnut-mcpをsystemdで常駐化し、OpenAI Secure MCP Tunnel経由で利用する手順です。

機能、MCPアクション、ローカルでの設定方法は[README](../README.md)を参照してください。

`tunnel-client`がwalnut-mcpをstdioの子プロセスとして起動します。MCP用の外部公開ポートとNginxは不要です。トンネルの管理画面も`127.0.0.1`の空きポートだけで待ち受けます。

## 構成

```text
ChatGPT / Codex
  ↓ OpenAI Secure MCP Tunnel
Server: tunnel-client
  ↓ stdio
walnut-mcp
  ↓ HTTPS
Discord API
```

VPS上の配置先です。

```text
/usr/local/bin/tunnel-client
/etc/systemd/system/walnut-mcp-tunnel.service
/srv/discord-bots/walnut-mcp/
├─ walnut-mcp
├─ .env
├─ persona.md
├─ tunnel.env
└─ tunnel-profiles/
   └─ walnut-mcp.yaml
```

`.env`はDiscordとMCPの設定、`persona.md`は投稿ペルソナ、`tunnel.env`はOpenAIのトンネル認証に使用します。いずれもGitへ追加しません。

## 1. Linuxバイナリの作成

WindowsのPowerShellで実行します。

```powershell
go test ./...
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o dist/walnut-mcp-linux-amd64 ./cmd/walnut-mcp
Get-FileHash -Algorithm SHA256 dist/walnut-mcp-linux-amd64
```

ビルド後、必要なら`GOOS`、`GOARCH`、`CGO_ENABLED`をPowerShellから削除します。

```powershell
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
```

## 2. VPSへの配置

SSH鍵、ホスト名、IPアドレスは実際の値へ置き換えます。READMEやGitの履歴には記録しません。

最初に、対話ログインできない専用system userと配置先を作成します。通常のSSHユーザーとサービスの実行ユーザーは共有しません。

```bash
useradd --system --user-group --home-dir /nonexistent \
  --shell /usr/sbin/nologin walnut-mcp
install -d -o root -g root -m 0751 /srv/discord-bots
install -d -o root -g walnut-mcp -m 0750 \
  /srv/discord-bots/walnut-mcp
```

```powershell
scp -i "<SSH_KEY>" dist/walnut-mcp-linux-amd64 root@<VPS_HOST>:/srv/discord-bots/walnut-mcp/walnut-mcp.new
scp -i "<SSH_KEY>" persona.md root@<VPS_HOST>:/srv/discord-bots/walnut-mcp/persona.md.new
ssh -i "<SSH_KEY>" root@<VPS_HOST>
```

VPSで所有者と権限を設定し、実行ファイルを配置します。

```bash
install -o root -g walnut-mcp -m 0750 \
  /srv/discord-bots/walnut-mcp/walnut-mcp.new \
  /srv/discord-bots/walnut-mcp/walnut-mcp.next
mv /srv/discord-bots/walnut-mcp/walnut-mcp.next \
  /srv/discord-bots/walnut-mcp/walnut-mcp
rm /srv/discord-bots/walnut-mcp/walnut-mcp.new
install -o root -g walnut-mcp -m 0640 \
  /srv/discord-bots/walnut-mcp/persona.md.new \
  /srv/discord-bots/walnut-mcp/persona.md
rm /srv/discord-bots/walnut-mcp/persona.md.new
```

## 3. walnut-mcpの環境変数

`/srv/discord-bots/walnut-mcp/.env`を作成します。値は例のまま使わず、実際の設定へ置き換えます。

```dotenv
DISCORD_BOT_TOKEN=<DISCORD_BOT_TOKEN>
DISCORD_CHANNEL_ID=<DISCORD_CHANNEL_ID>
DISCORD_API_BASE_URL=https://discord.com/api/v10
DISCORD_EMBED_COLOR=<#RRGGBB>
DISCORD_EMBED_THUMBNAIL_URL=<HTTPS_IMAGE_URL_OR_EMPTY>
MCP_PERSONA_FILE=persona.md
MCP_MESSAGE_SUFFIX=<OPTIONAL_MESSAGE_SUFFIX>
MCP_TRANSPORT=stdio
MCP_ADDR=
MCP_BEARER_TOKEN=
```

```bash
chown root:walnut-mcp /srv/discord-bots/walnut-mcp/.env
chmod 0640 /srv/discord-bots/walnut-mcp/.env
```

walnut-mcpは作業ディレクトリの`.env`と、`MCP_PERSONA_FILE`で指定した投稿ペルソナを読み込みます。stdioではTCPポートを待ち受けないため、`MCP_ADDR`と`MCP_BEARER_TOKEN`は空にします。

## 4. Secure MCP Tunnelの準備

[OpenAIのトンネル管理画面](https://platform.openai.com/settings/organization/tunnels)でトンネルを作成し、次を取得します。

- Tunnel ID
- Runtime API Key

公式の[tunnel-client](https://github.com/openai/tunnel-client/releases/tag/v0.0.11)をVPSへ配置します。この手順の動作確認済みバージョンは`v0.0.11`です。更新する場合はリリースノートを確認し、ダウンロードしたバイナリをリリース掲載のSHA-256と照合してからインストールします。

```bash
install -o root -g root -m 0755 tunnel-client /usr/local/bin/tunnel-client
/usr/local/bin/tunnel-client version
```

トンネル用の環境ファイルを作成します。

```dotenv
CONTROL_PLANE_TUNNEL_ID=<TUNNEL_ID>
CONTROL_PLANE_API_KEY=<RUNTIME_API_KEY>
```

```bash
chown root:root /srv/discord-bots/walnut-mcp/tunnel.env
chmod 0600 /srv/discord-bots/walnut-mcp/tunnel.env
install -d -o walnut-mcp -g walnut-mcp -m 0750 \
  /srv/discord-bots/walnut-mcp/tunnel-profiles
```

`tunnel.env`を`root:root 0600`にすると、サービス実行ユーザーから直接読み取れません。systemdが起動時に読み込み、必要な値だけをプロセスへ渡します。

## 5. トンネルプロファイルの作成

VPSのrootシェルで実行します。

```bash
set -a
. /srv/discord-bots/walnut-mcp/tunnel.env
set +a

runuser -u walnut-mcp -- env CONTROL_PLANE_API_KEY="$CONTROL_PLANE_API_KEY" \
  /usr/local/bin/tunnel-client init \
  --sample sample_mcp_stdio_local \
  --profile walnut-mcp \
  --profile-dir /srv/discord-bots/walnut-mcp/tunnel-profiles \
  --tunnel-id "$CONTROL_PLANE_TUNNEL_ID" \
  --mcp-command "/srv/discord-bots/walnut-mcp/walnut-mcp" \
  --health-listen-addr "127.0.0.1:0"
```

`127.0.0.1:0`はループバックの空きポートを自動選択します。8080を固定使用しないため、VPS上の既存サービスと競合しません。

生成後のプロファイルはサービスから読み取り専用にします。

```bash
chown root:walnut-mcp \
  /srv/discord-bots/walnut-mcp/tunnel-profiles \
  /srv/discord-bots/walnut-mcp/tunnel-profiles/walnut-mcp.yaml
chmod 0750 /srv/discord-bots/walnut-mcp/tunnel-profiles
chmod 0640 /srv/discord-bots/walnut-mcp/tunnel-profiles/walnut-mcp.yaml
```

作成したプロファイルを検証します。

```bash
runuser -u walnut-mcp -- env CONTROL_PLANE_API_KEY="$CONTROL_PLANE_API_KEY" \
  /usr/local/bin/tunnel-client doctor \
  --profile walnut-mcp \
  --profile-dir /srv/discord-bots/walnut-mcp/tunnel-profiles
```

すべての必須項目が`PASS`になり、最後に`RESULT ok`と表示されればプロファイルは有効です。

## 6. systemdで常駐化

リポジトリの`deploy/systemd/walnut-mcp-tunnel.service`をVPSへ送ります。

```powershell
scp -i "<SSH_KEY>" deploy/systemd/walnut-mcp-tunnel.service root@<VPS_HOST>:/tmp/walnut-mcp-tunnel.service
```

VPSでsystemdの管理ディレクトリへ配置します。

```bash
install -o root -g root -m 0644 \
  /tmp/walnut-mcp-tunnel.service \
  /etc/systemd/system/walnut-mcp-tunnel.service
rm /tmp/walnut-mcp-tunnel.service
systemctl daemon-reload
systemctl enable --now walnut-mcp-tunnel.service
```

稼働状態を確認します。

```bash
systemctl is-enabled walnut-mcp-tunnel.service
systemctl is-active walnut-mcp-tunnel.service
systemctl status walnut-mcp-tunnel.service --no-pager
journalctl -u walnut-mcp-tunnel.service -n 100 --no-pager
```

正常時はサービスが`active (running)`になり、ログに次が出ます。

- `stdio MCP command started`
- `Discord Gateway connected`
- `starting MCP server transport=stdio`
- `poller started`
- `tunnel-client started`

## 7. ChatGPTから接続

サービスが起動している状態で、ChatGPTの設定から開発者モードのアプリを作成します。

1. Connectionで`Tunnel`を選択します。
2. 作成済みトンネルを選択します。
3. Authenticationは`No authentication`を選択します。
4. MCPツール一覧に`send_discord_embed`と`read_recent_messages`が表示されることを確認します。
5. 最初はDiscordへの送信を伴わない`read_recent_messages`で疎通確認します。

現在のwalnut-mcpが公開するアクションは、この2つだけです。他のアクションが表示されなくても問題ありません。

`No authentication`は、個人用ワークスペースから非公開トンネルを利用する現在の構成に限った設定です。Runtime API Keyによるトンネル認証は別に機能しています。

トンネルが一覧へ出ない場合は、OpenAI Platform側で対象のChatGPTワークスペースとトンネルが関連付けられているか確認します。

## 更新

新しいバイナリを`walnut-mcp.new`としてアップロードし、SHA-256を確認してから入れ替えます。

```bash
sha256sum /srv/discord-bots/walnut-mcp/walnut-mcp.new
install -o root -g walnut-mcp -m 0750 \
  /srv/discord-bots/walnut-mcp/walnut-mcp.new \
  /srv/discord-bots/walnut-mcp/walnut-mcp.next
mv /srv/discord-bots/walnut-mcp/walnut-mcp.next \
  /srv/discord-bots/walnut-mcp/walnut-mcp
rm /srv/discord-bots/walnut-mcp/walnut-mcp.new
systemctl restart walnut-mcp-tunnel.service
systemctl is-active walnut-mcp-tunnel.service
```

`.env`を変更した場合もサービスを再起動します。トンネル自体を作り直す必要はありません。

## 停止と障害調査

```bash
systemctl stop walnut-mcp-tunnel.service
systemctl disable walnut-mcp-tunnel.service
journalctl -u walnut-mcp-tunnel.service --since "30 minutes ago" --no-pager
```

調査時も`cat .env`や`cat tunnel.env`を実行しないでください。ログや画面共有へ秘密情報が残る原因になります。

接続できない場合は、次の順で確認します。

1. `systemctl is-active`が`active`か
2. `doctor`が`RESULT ok`か
3. VPSから`api.openai.com:443`へ接続できるか
4. OpenAI Platformでトンネルとワークスペースが関連付けられているか
5. Discord Bot Tokenとチャンネル権限が有効か

## セキュリティ

- `walnut-mcp`は対話ログインさせず、SSHユーザーの所属グループにも追加しません。`.env`、`persona.md`、トンネルプロファイルは`root:walnut-mcp`で管理し、サービスだけが読み取れる状態にします。
- UFWにはSSH以外の受信許可を追加しません。
- Nginx、MCP用公開ポート、ポートフォワードは使用しません。
- `DISCORD_BOT_TOKEN`、Runtime API Key、SSH秘密鍵をREADME、Issue、ログへ貼りません。
- 秘密情報が漏れた可能性があれば、Discord Bot TokenとRuntime API Keyを直ちに再発行します。
- トンネル管理画面や料金設定はOpenAI側で変更される可能性があるため、利用開始前に対象アカウントの表示を確認します。

Secure MCP Tunnelの仕組みと最新手順は、[OpenAI公式ドキュメント](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)を参照してください。
