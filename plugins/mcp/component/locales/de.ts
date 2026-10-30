export const de: Record<string, string> = {
  "mcp.error.toolPolicy":
    "Die Werkzeugrichtlinie konnte nicht geändert werden.",
  "mcp.tools.policyHint":
    "Wird sofort gespeichert. Die Sichtbarkeit gilt für neue Ausführungen. Automatische Genehmigung setzt eine globale Erlaubnisregel; spezifischere Regeln gelten weiterhin.",
  "mcp.tools.nameConflict":
    "Ausgeblendet, weil {{name}} mit {{sources}} kollidiert.",
  "mcp.add": "Server hinzufügen",
  "mcp.save": "Speichern",
  "mcp.saving": "Speichern…",
  "mcp.delete": "Löschen",
  "mcp.delete.title": "Diesen Server löschen?",
  "mcp.delete.body":
    "„{{server}}“ und seine Tools werden entfernt. Du kannst ihn später wieder hinzufügen.",
  "mcp.test": "Testen",
  "mcp.testing": "Test läuft…",
  "mcp.connectionOk": "Verbindung OK",
  "mcp.toolCount_one": "{{count}} Tool",
  "mcp.toolCount_other": "{{count}} Tools",
  "mcp.empty": "Keine MCP-Server",
  "mcp.enable.aria": "{{server}} aktivieren",
  "mcp.edit": "{{server}} bearbeiten",
  "mcp.transport.stdio": "stdio",
  "mcp.transport.http": "Streamable HTTP",
  "mcp.signIn": "Anmelden",
  "mcp.signingIn": "Anmeldung läuft…",
  "mcp.signIn.aria": "Bei {{server}} anmelden",
  "mcp.signingIn.aria": "Anmeldung läuft… {{server}}",
  "mcp.reconnect": "Neu verbinden",
  "mcp.reconnect.aria": "{{server}} neu verbinden",
  "mcp.status.disabled": "Deaktiviert",
  "mcp.status.disconnected": "Nicht verbunden",
  "mcp.status.connecting": "Verbindung wird hergestellt…",
  "mcp.status.connected": "Verbunden",
  "mcp.status.failed": "Verbindung fehlgeschlagen",
  "mcp.status.needsAuth": "Anmeldung erforderlich",
  "mcp.form.name.aria": "Servername",
  "mcp.form.transport.aria": "Transport",
  "mcp.form.command.placeholder": "npx",
  "mcp.form.command.aria": "Befehl",
  "mcp.form.args": "Argumente (JSON-Array)",
  "mcp.form.args.placeholder":
    "-y\n@modelcontextprotocol/server-filesystem\n/pfad",
  "mcp.form.env": "Umgebung (JSON-Objekt)",
  "mcp.form.env.placeholder": "API_KEY=…",
  "mcp.form.env.keep":
    "Leer lassen, um die gespeicherte Umgebung beizubehalten",
  "mcp.form.env.clear": "Gespeicherte Umgebung entfernen",
  "mcp.form.env.targetChanged":
    "Das Prozessziel hat sich geändert. Eine neue Umgebung eingeben oder die gespeicherten Werte entfernen.",
  "mcp.form.dir.aria": "Arbeitsverzeichnis",
  "mcp.form.url.placeholder": "https://example.com/mcp",
  "mcp.form.url.aria": "Server-URL",
  "mcp.form.auth.placeholder": "Autorisierungstoken (optional)",
  "mcp.form.auth.keep": "Leer lassen, um das gespeicherte Token zu behalten",
  "mcp.form.auth.clear": "Gespeicherte Autorisierung entfernen",
  "mcp.form.auth.originChanged":
    "Der Ursprung des Endpunkts hat sich geändert. Neue Autorisierung eingeben oder den gespeicherten Wert entfernen.",
  "mcp.form.auth.aria": "Autorisierungstoken",
  "mcp.form.headers": "Header (JSON-Objekt)",
  "mcp.form.headers.placeholder": "X-API-Key=…",
  "mcp.form.headers.keep":
    "Leer lassen, um die gespeicherten Header beizubehalten",
  "mcp.form.headers.clear": "Gespeicherte Header entfernen",
  "mcp.form.headers.originChanged":
    "Der Ursprung des Endpunkts hat sich geändert. Neue Header eingeben oder die gespeicherten Werte entfernen.",
  "mcp.form.timeout": "Timeout (Sekunden) — optional",
  "mcp.form.timeout.placeholder": "60",
  "mcp.form.timeout.invalid":
    "Gib eine positive ganze Zahl an Sekunden ein oder lass das Feld leer für kein Zeitlimit.",
  "mcp.form.description.aria": "Beschreibung",
  "mcp.import": "Aus JSON importieren",
  "mcp.import.confirm": "Importieren",
  "mcp.importing": "Import läuft…",
  "mcp.import.ok_one": "{{count}} Server importiert",
  "mcp.import.ok_other": "{{count}} Server importiert",
  "mcp.import.renamed":
    "Umbenannt, damit die Runtime sie akzeptiert: {{names}}",
  "mcp.import.interrupted":
    "Runtime-Verbindung geändert: {{count}} von {{total}} Servern vor dem Abbruch importiert.",
  "mcp.import.partial": "{{count}} von {{total}} Servern importiert.",
  "mcp.import.error": "Diese Konfiguration konnte nicht importiert werden.",
  "mcp.import.nameCollision":
    "Die Server „{{first}}“ und „{{second}}“ werden beide zu „{{canonical}}“; gib ihnen unterschiedliche Namen",
  "mcp.import.invalidJson": "Gib gültiges JSON ein.",
  "mcp.import.invalidShape":
    "Verwende eine gültige MCP-Konfiguration mit einem „mcpServers“-Objekt.",
  "mcp.import.invalidName":
    "Der Servername „<server/>“ entspricht nicht den Namensregeln der Runtime. Ändere den Schlüssel und versuche es erneut.",
  "mcp.import.missingTransport":
    "Gib für Server „<server/>“ entweder einen Befehl oder eine URL an.",
  "mcp.import.missingCommand":
    "Füge für den Server „<server/>“ einen Befehl hinzu.",
  "mcp.import.missingUrl": "Füge für den Server „<server/>“ eine URL hinzu.",
  "mcp.import.invalidTimeout":
    "Lege für den Server „<server/>“ ein Timeout in positiven ganzen Sekunden fest oder lass es weg für kein Zeitlimit.",
  "mcp.import.empty": "Füge unter „mcpServers“ mindestens einen Server hinzu.",
  "mcp.tools.title": "Tools von {{server}}",
  "mcp.tools.enabled": "Aktiviert",
  "mcp.tools.autoApprove": "Automatisch genehmigen",
  "mcp.tools.empty": "Dieser Server stellt keine Tools bereit",
  "mcp.tools.enable.aria": "{{tool}} aktivieren",
  "mcp.tools.autoApprove.aria": "{{tool}} automatisch genehmigen",
  "mcp.error.save": "Server konnte nicht gespeichert werden.",
  "mcp.error.remove": "Server konnte nicht entfernt werden.",
  "mcp.error.toggle": "Server konnte nicht geändert werden.",
  "mcp.error.signIn": "Anmeldung fehlgeschlagen.",
  "mcp.error.reconnect": "Server konnte nicht neu verbunden werden.",
  "mcp.error.test": "Test fehlgeschlagen",
  "mcp.error.loadTools": "Tools konnten nicht geladen werden",
  "mcp.error.loadToolVisibility":
    "Tool-Sichtbarkeit konnte nicht geladen werden",
  "mcp.error.loadAutoApproveRules":
    "Regeln für automatische Genehmigung konnten nicht geladen werden",
  "mcp.testOutcome.timedOut":
    "Der Server hat nicht rechtzeitig geantwortet – prüfe Befehl oder URL und versuche es erneut.",
  "mcp.testOutcome.failed":
    "Der Server hat keine Verbindung hergestellt oder keine gültige Werkzeugliste geliefert. Prüfe die Einstellungen und versuche es erneut.",
  "mcp.import.config": "JSON-Konfiguration",
  "mcpStatus.mcp_authorization_required":
    "Dieser Server verlangt eine Anmeldung, bevor er nutzbar ist.",
  "mcpStatus.mcp_authorization_failed":
    "Die Anmeldung wurde nicht abgeschlossen. Versuche es erneut.",
  "mcpStatus.mcp_tool_discovery_failed":
    "Verbunden, aber dieser Server hat keine verwendbare Werkzeugliste geliefert. Prüfe den Server und versuche es erneut.",
  "mcpStatus.mcp_configuration_failed":
    "Die Konfiguration oder die gespeicherten Anmeldedaten dieses Servers konnten nicht verwendet werden. Prüfe die Einstellungen und versuche es erneut.",
  "mcpStatus.mcp_dial_failed":
    "Dieser Server ist nicht erreichbar — prüfe Befehl oder URL und versuche es erneut.",
  "common.cancel": "Abbrechen",
  "common.close": "Schließen",
  "command.uncertain":
    "Das Ergebnis des ursprünglichen Befehls ist nicht bestätigt. Gleiche es ab, bevor du einen weiteren Befehl ausführst oder den Katalog wechselst.",
  "command.reconcile": "Ursprünglichen Befehl abgleichen",
  "read.retained": "Die letzte akzeptierte Lesung bleibt erhalten.",
  "mcp.form.args.invalid":
    "Gib ein JSON-Array mit Zeichenfolgen ein, oder lasse das Feld ohne Argumente leer.",
  "mcp.form.env.invalid":
    "Gib ein nicht leeres JSON-Objekt mit Zeichenfolgen als Werten ein.",
  "mcp.form.headers.invalid":
    "Gib ein nicht leeres JSON-Objekt mit Zeichenfolgen als Werten ein.",
  "mcp.import.unsupportedTransport":
    "Nicht unterstützter „type“ für Server „<server/>“. Verwende „stdio“ oder „streamableHttp“.",
  "mcp.import.headerConflict":
    "Server „<server/>“ deklariert „<header/>“ mehrfach. Behalte eine Deklaration.",
};
