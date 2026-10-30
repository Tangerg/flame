export const ja: Record<string, string> = {
  "providers.apiKey.replace": "API キーを置き換え…",
  "providers.apiKey.clear": "保存済みキーを削除",
  "providers.apiKey.clear.title": "保存済みキーを削除しますか？",
  "providers.apiKey.clear.body":
    "{{provider}} の保存済み API キーが失われます。再接続するには貼り付け直す必要があります。",
  "providers.apiKey.placeholder": "API キー",
  "providers.apiKey.aria": "{{provider}} の API キー",
  "providers.baseUrl.placeholder": "ベース URL",
  "providers.baseUrl.aria": "{{provider}} のベース URL",
  "providers.key": "キー {{masked}}",
  "providers.ready": "利用可能",
  "providers.notConfigured": "未設定",
  "providers.save": "保存",
  "providers.saving": "保存中…",
  "providers.test": "テスト",
  "providers.testing": "テスト中…",
  "providers.connectionOk": "接続 OK",
  "providers.error.save": "保存に失敗しました",
  "providers.error.test": "テストに失敗しました",
  "providers.models.error": "モデルを読み込めませんでした。",
  "providers.models.refresh": "モデル一覧を更新",
  "providers.empty": "プロバイダーがありません",
  "providers.empty.sub":
    "ランタイムはサポートされている LLM プロバイダーを報告していません。",
  "providers.apiKey.envPlaceholder":
    "環境変数のキーを使用中 — 入力すると上書きします",
  "providers.fromEnv": "環境変数から",
  "providers.utility.title": "補助モデル",
  "providers.utility.desc":
    "要約、タイトル、事実抽出に使うモデルです。未設定時は Runtime の既定モデルを使用します。変更は実行中の処理を含め、次の補助呼び出しから反映されます。",
  "providers.utility.main": "Runtime の既定モデルを使用",
  "providers.utility.error": "補助モデルを設定できませんでした。",
  "providers.embedding.title": "埋め込みモデル",
  "providers.embedding.desc":
    "Agent Memory のランキングを必要に応じて改善します。オフの場合も Memory はキーワード照合を続けます。 以降の検索に適用されます。",
  "providers.embedding.off": "オフ",
  "providers.embedding.none":
    "埋め込み対応のプロバイダーがまだ設定されていません — 上で追加してください（例：OpenAI）。",
  "providers.embedding.error": "埋め込みモデルを設定できませんでした。",
  "providers.testOutcome.notConfigured":
    "先にプロバイダーの設定を完了してください。",
  "providers.testOutcome.failed":
    "プロバイダーに接続できないか、テストリクエストが拒否されました。",
  "providers.apiKey.label": "API キー",
  "providers.baseUrl.label": "ベース URL",
  "providers.baseUrl.labelRequired": "ベース URL · 必須",
  "providers.baseUrl.required":
    "このプロバイダーを保存するにはベース URL が必要です。",
  "providers.saveAndTest": "保存してテスト",
  "providers.saved": "保存しました",
  "providers.role.loadError":
    "この設定を読み込めないため、現在の値は不明です。",
  "providers.role.stale":
    "最後に取得した値を表示しています（更新に失敗しました）。",
  "providers.setup.steps":
    "プロバイダーを 1 つ選び、API キーを貼り付けて「保存してテスト」を押してください。始めるにはこれだけで十分です。",
  "providers.setup.done":
    "プロバイダーの準備ができました。設定を閉じると戻れます。下書きはそのままです。",
  "providers.roles.heading": "モデルの役割",
  "rpcError.invalid_api_key":
    "プロバイダが API キーを拒否しました — プロバイダ設定で確認してください。",
  "rpcError.timeout":
    "モデルプロバイダへのリクエストがタイムアウトしました — 接続を確認して再試行してください。",
  title: "プロバイダー",
  cancel: "キャンセル",
  reading: "読み込み中",
  refresh: "更新",
  retry: "再試行",
  "command.uncertain":
    "元のコマンドの結果は未確認です。別のコマンドを実行したりカタログを切り替えたりする前に照合してください。",
  "command.reconcile": "元のコマンドを照合",
  "read.retained": "最後に受け入れた読み取りを保持しています。",
  close: "閉じる",
  "providers.embedding.noPreset":
    "設定済みのプロバイダーには、プリセットの埋め込みモデルがありません。",
};
