export const ko: Record<string, string> = {
  "providers.apiKey.replace": "API 키 교체…",
  "providers.apiKey.clear": "저장된 키 제거",
  "providers.apiKey.clear.title": "저장된 키를 제거할까요?",
  "providers.apiKey.clear.body":
    "{{provider}}의 저장된 API 키가 사라집니다. 다시 연결하려면 키를 다시 붙여넣어야 합니다.",
  "providers.apiKey.placeholder": "API 키",
  "providers.apiKey.aria": "{{provider}} API 키",
  "providers.baseUrl.placeholder": "기본 URL",
  "providers.baseUrl.aria": "{{provider}} 기본 URL",
  "providers.key": "키 {{masked}}",
  "providers.ready": "사용 가능",
  "providers.notConfigured": "미설정",
  "providers.save": "저장",
  "providers.saving": "저장 중…",
  "providers.test": "테스트",
  "providers.testing": "테스트 중…",
  "providers.connectionOk": "연결 정상",
  "providers.error.save": "저장 실패",
  "providers.error.test": "테스트 실패",
  "providers.models.error": "모델을 불러올 수 없습니다.",
  "providers.models.refresh": "모델 목록 새로고침",
  "providers.empty": "공급자 없음",
  "providers.empty.sub": "런타임이 지원되는 LLM 공급자를 보고하지 않습니다.",
  "providers.apiKey.envPlaceholder":
    "환경 변수 키 사용 중 — 입력하면 덮어씁니다",
  "providers.fromEnv": "환경 변수에서",
  "providers.utility.title": "보조 모델",
  "providers.utility.desc":
    "요약, 제목, 사실 추출용 모델입니다. 미설정 시 Runtime 기본 모델을 사용합니다. 변경 사항은 진행 중인 실행에서도 다음 보조 호출부터 적용됩니다.",
  "providers.utility.main": "Runtime 기본 모델 사용",
  "providers.utility.error": "보조 모델을 설정할 수 없습니다.",
  "providers.embedding.title": "임베딩 모델",
  "providers.embedding.desc":
    "Agent Memory 순위를 선택적으로 개선합니다. 꺼도 Memory는 키워드 일치를 계속 사용합니다. 이후 검색에 적용됩니다.",
  "providers.embedding.off": "끄기",
  "providers.embedding.none":
    "임베딩을 지원하는 제공자가 아직 없습니다 — 위에서 하나 추가하세요(예: OpenAI).",
  "providers.embedding.error": "임베딩 모델을 설정할 수 없습니다.",
  "providers.testOutcome.notConfigured": "먼저 제공자 구성을 완료하세요.",
  "providers.testOutcome.failed":
    "제공자에 연결할 수 없거나 테스트 요청이 거부됐습니다.",
  "providers.apiKey.label": "API 키",
  "providers.baseUrl.label": "기본 URL",
  "providers.baseUrl.labelRequired": "기본 URL · 필수",
  "providers.baseUrl.required": "이 공급자를 저장하려면 기본 URL이 필요합니다.",
  "providers.saveAndTest": "저장 후 테스트",
  "providers.saved": "저장됨",
  "providers.role.loadError":
    "이 설정을 불러올 수 없어 현재 값을 알 수 없습니다.",
  "providers.role.stale":
    "마지막으로 확인된 값을 표시 중입니다 — 새로 고침에 실패했습니다.",
  "providers.setup.steps":
    "공급자 하나를 고르고 API 키를 붙여넣은 뒤 ‘저장 후 테스트’를 누르세요. 시작하려면 이것만 하면 됩니다.",
  "providers.setup.done":
    "공급자가 준비되었습니다. 설정을 닫고 돌아가면 초안이 그대로 있습니다.",
  "providers.roles.heading": "모델 역할",
  "rpcError.invalid_api_key":
    "제공자가 API 키를 거부했습니다 — 제공자 설정에서 확인하세요.",
  "rpcError.timeout":
    "모델 제공자 요청이 시간 초과되었습니다 — 연결을 확인하고 다시 시도하세요.",
  title: "공급자",
  cancel: "취소",
  reading: "불러오는 중",
  refresh: "새로 고침",
  retry: "다시 시도",
  "command.uncertain":
    "원래 명령의 결과가 확인되지 않았습니다. 다른 명령을 실행하거나 카탈로그를 전환하기 전에 조정하세요.",
  "command.reconcile": "원래 명령 조정",
  "read.retained": "마지막으로 수락된 읽기를 유지합니다.",
  close: "닫기",
  "providers.embedding.noPreset":
    "구성된 공급자에 사전 설정된 임베딩 모델이 없습니다.",
};
