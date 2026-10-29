export const de: Record<string, string> = {
  "panel.title": "Gespeicherte Nutzungsberichte",
  "panel.description":
    "Unveränderliche Kopien der Kernabrechnungsberichte. Die Live-Abrechnung bleibt unberührt.",
  "period.daily.label": "Täglich",
  "period.daily.window": "letzte 24 Stunden",
  "period.weekly.label": "Wöchentlich",
  "period.weekly.window": "letzte 7 Tage",
  "report.title": "Bericht ({{period}}) · {{observed}}",
  "row.title": "{{period}} · {{observed}}",
  "period.windowed": "{{label}} · {{window}}",
  "row.description": "{{window}} · Beginnt {{since}}",
  "report.scope.label": "Gespeicherter Bericht",
  "report.refresh": "Bericht aktualisieren",
  "report.back": "Zurück zum Archiv",
  "archive.actions.label": "Archivaktionen",
  "capture.period.label": "Erfassungszeitraum",
  "capture.save": "Bericht speichern",
  "archive.refresh": "Archiv aktualisieren",
  "evidence.beginning": "Beginn der aufbewahrten Nachweise",
  "cost.notReported": "Nicht gemeldet",
  "pending.label": "Ursprünglicher Archivbefehl",
  "pending.note": "Bestätigung für den ursprünglichen Befehl ausstehend.",
  "pending.check": "Ursprüngliches Ergebnis prüfen",
  "pending.retire": "Ursprünglichen Befehl zurückziehen",
  "archive.title": "Archiv",
  "archive.count_one":
    "{{count}} gespeicherter Bericht. Die Kapazität beträgt {{capacity}}; entferne einen Bericht, um Platz zu schaffen.",
  "archive.count_other":
    "{{count}} gespeicherte Berichte. Die Kapazität beträgt {{capacity}}; entferne einen Bericht, um Platz zu schaffen.",
  "archive.capture.note":
    "Eine Erfassung kopiert vor ihrer Ausführung die Kernabrechnung der {{daily}} (täglich) oder der {{weekly}} (wöchentlich), nicht eines Kalendertags oder einer Kalenderwoche.",
  "archive.reading": "Gespeicherte Berichte werden gelesen…",
  "archive.error": "Das Archiv konnte nicht gelesen werden: {{error}}.",
  "archive.error.retained":
    "Das Archiv konnte nicht gelesen werden: {{error}}. Das zuletzt akzeptierte Archiv wird weiterhin angezeigt.",
  "archive.empty.title": "Keine gespeicherten Berichte",
  "archive.empty.sub":
    "Wähle einen Zeitraum und speichere den aktuellen Kernbericht.",
  "archive.list.label": "Gespeicherte Berichte",
  "report.view": "Bericht anzeigen",
  "report.view.aria": "Bericht anzeigen: {{title}}",
  "removal.title": "{{title}} entfernen?",
  "removal.description":
    "Nur diese gespeicherte Kopie wird entfernt. Nachweise und Summen der Kernabrechnung bleiben unverändert, und die Kopie kann nicht wiederhergestellt werden.",
  "removal.close": "Entfernungsbestätigung schließen",
  "removal.cancel": "Abbrechen",
  "report.remove": "Gespeicherten Bericht entfernen",
  "removal.removing": "Bericht wird entfernt…",
  "fact.period": "Zeitraum",
  "fact.observation": "Kernbeobachtung",
  "fact.lowerBound": "Tatsächliche Untergrenze",
  "report.saved.note":
    "Gespeicherte Kopie eines Kernabrechnungsberichts. Sie wird nicht aktualisiert, wenn sich die Live-Abrechnung ändert.",
  "report.reading": "Gespeicherter Bericht wird gelesen…",
  "report.error.refresh":
    "Der gespeicherte Bericht konnte nicht aktualisiert werden: {{error}}. Der zuletzt akzeptierte Bericht bleibt sichtbar.",
  "report.error.read":
    "Der gespeicherte Bericht konnte nicht gelesen werden: {{error}}.",
  "report.totals.title": "Gespeicherte Kernsummen",
  "report.totals.description":
    "Die Werte sind die gespeicherte Kernantwort. Unbekannte Nutzung und Preise behalten ihre Kernbedeutung.",
  "fact.attempts": "Modellversuche",
  "fact.inputTokens": "Bekannte Eingabe-Tokens",
  "fact.outputTokens": "Bekannte Ausgabe-Tokens",
  "fact.knownSubtotal": "Zwischensumme bekannter Preise (USD)",
  "fact.totalCost": "Gesamtkosten (USD)",
  "fact.usageRecorded": "Nutzung erfasst",
  "fact.pricedAttempts": "Versuche mit Preis",
  "fact.outcomeUnknown": "Ergebnis unbekannt",
  "report.breakdown.title": "Tägliche Kernaufschlüsselung",
  "report.breakdown.row": "{{attempts}} Versuche · {{cost}} USD bekannt",
  "report.identity.title": "Genaue Berichtsidentität",
  "fact.reportId": "Berichts-ID",
  "fact.observedAt": "Beobachtet um (UTC)",
  "fact.lowerBoundUtc": "Untergrenze (UTC)",
  "report.actions.label": "Aktionen für gespeicherten Bericht",
  "message.saved": "Kernbericht gespeichert.",
  "message.savedArchived":
    "Kernbericht gespeichert: {{title}}. Er ist im Archiv aufgeführt.",
  "message.removed":
    "Gespeicherter Bericht entfernt. Die Kernabrechnung bleibt unverändert.",
  "message.removalConfirmed":
    "Entfernung des ursprünglichen Berichts bestätigt. Die Kernabrechnung bleibt unverändert.",
  "message.unknown":
    "Die Bestätigung ist unbekannt. Prüfe das ursprüngliche Ergebnis, bevor du eine weitere Archivänderung sendest.",
  "message.archiveFailed": "Die Archivänderung ist fehlgeschlagen: {{error}}",
  "message.captureRejected":
    "Die Erfassung konnte nicht zugelassen werden: {{error}}",
  "message.removalRejected":
    "Die Entfernung konnte nicht zugelassen werden: {{error}}",
  "message.retired":
    "Ursprünglicher Befehl zurückgezogen. Bereits gespeicherte Berichte bleiben im Archiv.",
  "message.retireFailed":
    "Der ursprüngliche Befehl konnte nicht zurückgezogen werden: {{error}}",
};
