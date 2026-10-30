export const de: Record<string, string> = {
  "providers.apiKey.replace": "API-Schlüssel ersetzen…",
  "providers.apiKey.clear": "Gespeicherten Schlüssel entfernen",
  "providers.apiKey.clear.title": "Gespeicherten Schlüssel entfernen?",
  "providers.apiKey.clear.body":
    "{{provider}} verliert seinen gespeicherten API-Schlüssel. Zum erneuten Verbinden musst du ihn wieder einfügen.",
  "providers.apiKey.placeholder": "API-Schlüssel",
  "providers.apiKey.aria": "{{provider}} API-Schlüssel",
  "providers.baseUrl.placeholder": "Basis-URL",
  "providers.baseUrl.aria": "{{provider}} Basis-URL",
  "providers.key": "Schlüssel {{masked}}",
  "providers.ready": "bereit",
  "providers.notConfigured": "nicht konfiguriert",
  "providers.save": "Speichern",
  "providers.saving": "Speichern…",
  "providers.test": "Testen",
  "providers.testing": "Test läuft…",
  "providers.connectionOk": "Verbindung OK",
  "providers.error.save": "Speichern fehlgeschlagen",
  "providers.error.test": "Test fehlgeschlagen",
  "providers.models.error": "Modelle konnten nicht geladen werden.",
  "providers.models.refresh": "Modelle aktualisieren",
  "providers.empty": "Keine Anbieter",
  "providers.empty.sub": "Die Runtime meldet keine unterstützten LLM-Anbieter.",
  "providers.apiKey.envPlaceholder":
    "Umgebungsschlüssel wird verwendet — tippen zum Überschreiben",
  "providers.fromEnv": "aus Umgebung",
  "providers.utility.title": "Hilfsmodell",
  "providers.utility.desc":
    "Modell für Zusammenfassungen, Titel und Faktenextraktion. Ohne Auswahl gilt der Runtime-Standard. Änderungen gelten ab dem nächsten Hilfsaufruf, auch innerhalb einer laufenden Ausführung.",
  "providers.utility.main": "Runtime-Standard verwenden",
  "providers.utility.error": "Hilfsmodell konnte nicht gesetzt werden.",
  "providers.embedding.title": "Embedding-Modell",
  "providers.embedding.desc":
    "Verbessert optional das Ranking des Agent Memory. Bei Aus arbeitet Memory weiterhin mit Stichwortabgleich. Gilt für nachfolgende Suchen.",
  "providers.embedding.off": "Aus",
  "providers.embedding.none":
    "Noch kein Anbieter mit Embedding-Unterstützung konfiguriert — füge oben einen hinzu (z. B. OpenAI).",
  "providers.embedding.error": "Embedding-Modell konnte nicht gesetzt werden.",
  "providers.testOutcome.notConfigured":
    "Schließe zuerst die Provider-Konfiguration ab.",
  "providers.testOutcome.failed":
    "Der Anbieter war nicht erreichbar oder hat die Testanfrage abgelehnt.",
  "providers.apiKey.label": "API-Schlüssel",
  "providers.baseUrl.label": "Basis-URL",
  "providers.baseUrl.labelRequired": "Basis-URL · erforderlich",
  "providers.baseUrl.required":
    "Dieser Anbieter braucht eine Basis-URL, bevor er gespeichert werden kann.",
  "providers.saveAndTest": "Speichern & testen",
  "providers.saved": "Gespeichert",
  "providers.role.loadError":
    "Diese Einstellung konnte nicht geladen werden, ihr aktueller Wert ist unbekannt.",
  "providers.role.stale":
    "Zeigt den zuletzt bekannten Wert – die Aktualisierung ist fehlgeschlagen.",
  "providers.setup.steps":
    "Wähle einen Anbieter, füge seinen API-Schlüssel ein und klicke auf „Speichern & testen“. Mehr brauchst du zum Starten nicht.",
  "providers.setup.done":
    "Ein Anbieter ist bereit. Schließe die Einstellungen, um zurückzukehren – dein Entwurf ist noch da.",
  "providers.roles.heading": "Modellrollen",
  "rpcError.invalid_api_key":
    "Der Anbieter hat den API-Key abgelehnt — prüfe ihn in den Anbieter-Einstellungen.",
  "rpcError.timeout":
    "Die Anfrage an den Modellanbieter hat das Zeitlimit überschritten — Verbindung prüfen und erneut versuchen.",
  title: "Anbieter",
  cancel: "Abbrechen",
  reading: "Wird geladen",
  refresh: "Aktualisieren",
  retry: "Erneut versuchen",
  "command.uncertain":
    "Das Ergebnis des ursprünglichen Befehls ist nicht bestätigt. Gleiche es ab, bevor du einen weiteren Befehl ausführst oder den Katalog wechselst.",
  "command.reconcile": "Ursprünglichen Befehl abgleichen",
  "read.retained": "Die letzte akzeptierte Lesung bleibt erhalten.",
  close: "Schließen",
  "providers.embedding.noPreset":
    "Für die konfigurierten Anbieter ist kein voreingestelltes Embedding-Modell verfügbar.",
};
