export const ja: Record<string, string> = {
  "mcp.error.toolPolicy": "ツールのポリシーを更新できませんでした。",
  "mcp.tools.policyHint":
    "即座に保存されます。ツールの表示設定は新しい実行に適用されます。自動承認はグローバルな許可ルールを設定します。より具体的なルールも適用されます。",
  "mcp.tools.nameConflict":
    "{{name}} が {{sources}} と競合するため非表示です。",
  "mcp.add": "サーバーを追加",
  "mcp.save": "保存",
  "mcp.saving": "保存中…",
  "mcp.delete": "削除",
  "mcp.delete.title": "このサーバーを削除しますか？",
  "mcp.delete.body":
    "「{{server}}」とそのツールが削除されます。後で再び追加できます。",
  "mcp.test": "テスト",
  "mcp.testing": "テスト中…",
  "mcp.connectionOk": "接続は正常です",
  "mcp.toolCount_other": "{{count}} 個のツール",
  "mcp.empty": "MCP サーバーがありません",
  "mcp.enable.aria": "{{server}} を有効化",
  "mcp.edit": "{{server}} を編集",
  "mcp.transport.stdio": "stdio",
  "mcp.transport.http": "Streamable HTTP",
  "mcp.signIn": "サインイン",
  "mcp.signingIn": "サインイン中…",
  "mcp.signIn.aria": "{{server}} にサインイン",
  "mcp.signingIn.aria": "{{server}} にサインイン中…",
  "mcp.reconnect": "再接続",
  "mcp.reconnect.aria": "{{server}} に再接続",
  "mcp.status.disabled": "無効",
  "mcp.status.disconnected": "未接続",
  "mcp.status.connecting": "接続中…",
  "mcp.status.connected": "接続済み",
  "mcp.status.failed": "接続に失敗しました",
  "mcp.status.needsAuth": "サインインが必要です",
  "mcp.form.name.aria": "サーバー名",
  "mcp.form.transport.aria": "トランスポート",
  "mcp.form.command.placeholder": "npx",
  "mcp.form.command.aria": "コマンド",
  "mcp.form.args": "引数 — 1行に1つ",
  "mcp.form.args.placeholder":
    "-y\n@modelcontextprotocol/server-filesystem\n/path",
  "mcp.form.env": "環境変数 — KEY=value、1行に1つ",
  "mcp.form.env.placeholder": "API_KEY=…",
  "mcp.form.env.keep": "空欄のままにすると保存済みの環境変数を保持します",
  "mcp.form.env.clear": "保存済みの環境変数を削除",
  "mcp.form.env.targetChanged":
    "プロセスの対象が変わりました。新しい環境変数を入力するか、保存済みの値を削除してください。",
  "mcp.form.dir.aria": "作業ディレクトリ",
  "mcp.form.url.placeholder": "https://example.com/mcp",
  "mcp.form.url.aria": "サーバー URL",
  "mcp.form.auth.placeholder": "認証トークン（任意）",
  "mcp.form.auth.keep": "空欄のままにすると保存済みのトークンを保持します",
  "mcp.form.auth.clear": "保存済みの認証情報を削除",
  "mcp.form.auth.originChanged":
    "エンドポイントのオリジンが変わりました。新しい認証情報を入力するか、保存済みの値を削除してください。",
  "mcp.form.auth.aria": "認証トークン",
  "mcp.form.headers": "ヘッダー — KEY=value、1行に1つ",
  "mcp.form.headers.placeholder": "X-API-Key=…",
  "mcp.form.headers.keep": "空欄のままにすると保存済みのヘッダーを保持します",
  "mcp.form.headers.clear": "保存済みのヘッダーを削除",
  "mcp.form.headers.originChanged":
    "エンドポイントのオリジンが変わりました。新しいヘッダーを入力するか、保存済みの値を削除してください。",
  "mcp.form.timeout": "タイムアウト（秒）— 任意",
  "mcp.form.timeout.placeholder": "60",
  "mcp.form.timeout.invalid":
    "正の整数で秒数を入力するか、時間制限なしにする場合は空欄のままにしてください。",
  "mcp.form.description.aria": "説明",
  "mcp.import": "JSON からインポート",
  "mcp.import.confirm": "インポート",
  "mcp.importing": "インポート中…",
  "mcp.import.ok_other": "{{count}} 個のサーバーをインポートしました",
  "mcp.import.renamed": "ランタイムが受け付ける形式に改名しました: {{names}}",
  "mcp.import.interrupted":
    "Runtime 接続が変更されたため中断しました。{{total}} 件中 {{count}} 件をインポートしました。",
  "mcp.import.partial": "{{total}} 件中 {{count}} 件をインポートしました。",
  "mcp.import.error": "その設定をインポートできませんでした。",
  "mcp.import.nameCollision":
    "サーバー「{{first}}」と「{{second}}」はどちらも「{{canonical}}」になります。別々の名前を付けてください",
  "mcp.import.invalidJson": "有効な JSON を入力してください。",
  "mcp.import.invalidShape":
    "「mcpServers」オブジェクトを含む有効な MCP 設定を使用してください。",
  "mcp.import.invalidName":
    "サーバー「{{server}}」には a–z または 0–9 を含む名前を付けてください。",
  "mcp.import.missingTransport":
    "サーバー「{{server}}」には、コマンド付きの stdio または URL 付きの Streamable HTTP を指定してください。",
  "mcp.import.missingCommand":
    "サーバー「{{server}}」にコマンドを追加してください。",
  "mcp.import.missingUrl": "サーバー「{{server}}」に URL を追加してください。",
  "mcp.import.invalidTimeout":
    "サーバー「{{server}}」のタイムアウトを正の整数の秒数で設定するか、時間制限なしにする場合は省略してください。",
  "mcp.import.empty":
    "「mcpServers」の下にサーバーを 1 つ以上追加してください。",
  "mcp.tools.title": "{{server}} のツール",
  "mcp.tools.enabled": "有効",
  "mcp.tools.autoApprove": "自動承認",
  "mcp.tools.empty": "このサーバーはツールを公開していません",
  "mcp.tools.enable.aria": "{{tool}} を有効化",
  "mcp.tools.autoApprove.aria": "{{tool}} を自動承認",
  "mcp.error.save": "サーバーを保存できませんでした。",
  "mcp.error.remove": "サーバーを削除できませんでした。",
  "mcp.error.toggle": "サーバーを変更できませんでした。",
  "mcp.error.signIn": "サインインに失敗しました。",
  "mcp.error.reconnect": "サーバーに再接続できませんでした。",
  "mcp.error.test": "テストに失敗しました",
  "mcp.error.loadTools": "ツールを読み込めませんでした",
  "mcp.error.loadToolVisibility": "ツールの表示設定を読み込めませんでした",
  "mcp.error.loadAutoApproveRules": "自動承認ルールを読み込めませんでした",
  "mcp.testOutcome.timedOut":
    "サーバーが時間内に応答しませんでした。コマンドまたは URL を確認して再試行してください。",
  "mcp.testOutcome.failed":
    "サーバーに接続できないか、有効なツール一覧が返されませんでした。設定を確認して再試行してください。",
  "mcp.import.config": "JSON 設定",
  "mcpStatus.mcp_authorization_required":
    "このサーバーは使う前にサインインが必要です。",
  "mcpStatus.mcp_authorization_failed":
    "サインインが完了しませんでした。もう一度お試しください。",
  "mcpStatus.mcp_tool_discovery_failed":
    "接続しましたが、このサーバーは使用可能なツール一覧を返しませんでした。サーバーを確認して再試行してください。",
  "mcpStatus.mcp_configuration_failed":
    "このサーバーの設定または保存済みの認証情報を使用できません。設定を確認して再試行してください。",
  "mcpStatus.mcp_dial_failed":
    "このサーバーに接続できません — コマンドか URL を確認して再試行してください。",
  "common.cancel": "キャンセル",
  "common.close": "閉じる",
  title: "MCP サーバー",
  "command.uncertain":
    "元のコマンドの結果は未確認です。別のコマンドを実行したりカタログを切り替えたりする前に照合してください。",
  "command.reconcile": "元のコマンドを照合",
  "read.retained": "最後に受け入れた読み取りを保持しています。",
  reading: "読み込み中",
  refresh: "更新",
  retry: "再試行",
};
