export const es: Record<string, string> = {
  "mcp.error.toolPolicy":
    "No se pudo actualizar la política de la herramienta.",
  "mcp.tools.policyHint":
    "Se guarda al instante. La visibilidad se aplica a las nuevas ejecuciones. La aprobación automática establece una regla global de permiso; las reglas más específicas siguen vigentes.",
  "mcp.tools.nameConflict":
    "Oculta porque {{name}} entra en conflicto con {{sources}}.",
  "mcp.add": "Añadir servidor",
  "mcp.save": "Guardar",
  "mcp.saving": "Guardando…",
  "mcp.delete": "Eliminar",
  "mcp.delete.title": "¿Eliminar este servidor?",
  "mcp.delete.body":
    "«{{server}}» y sus herramientas se eliminan. Puedes volver a añadirlo más tarde.",
  "mcp.test": "Probar",
  "mcp.testing": "Probando…",
  "mcp.connectionOk": "Conexión correcta",
  "mcp.toolCount_one": "{{count}} herramienta",
  "mcp.toolCount_many": "{{count}} herramientas",
  "mcp.toolCount_other": "{{count}} herramientas",
  "mcp.empty": "No hay servidores MCP",
  "mcp.enable.aria": "Activar {{server}}",
  "mcp.edit": "Editar {{server}}",
  "mcp.transport.stdio": "stdio",
  "mcp.transport.http": "Streamable HTTP",
  "mcp.signIn": "Iniciar sesión",
  "mcp.signingIn": "Iniciando sesión…",
  "mcp.signIn.aria": "Iniciar sesión en {{server}}",
  "mcp.signingIn.aria": "Iniciando sesión en {{server}}…",
  "mcp.reconnect": "Reconectar",
  "mcp.reconnect.aria": "Reconectar {{server}}",
  "mcp.status.disabled": "Desactivado",
  "mcp.status.disconnected": "Desconectado",
  "mcp.status.connecting": "Conectando…",
  "mcp.status.connected": "Conectado",
  "mcp.status.failed": "Error de conexión",
  "mcp.status.needsAuth": "Inicio de sesión necesario",
  "mcp.form.name.aria": "Nombre del servidor",
  "mcp.form.transport.aria": "Transporte",
  "mcp.form.command.placeholder": "npx",
  "mcp.form.command.aria": "Comando",
  "mcp.form.args": "Argumentos (matriz JSON)",
  "mcp.form.args.placeholder":
    "-y\n@modelcontextprotocol/server-filesystem\n/ruta",
  "mcp.form.env": "Entorno (objeto JSON)",
  "mcp.form.env.placeholder": "API_KEY=…",
  "mcp.form.env.keep": "Déjalo en blanco para conservar el entorno guardado",
  "mcp.form.env.clear": "Eliminar el entorno guardado",
  "mcp.form.env.targetChanged":
    "El destino del proceso cambió. Introduce un entorno nuevo o elimina los valores guardados.",
  "mcp.form.dir.aria": "Directorio de trabajo",
  "mcp.form.url.placeholder": "https://example.com/mcp",
  "mcp.form.url.aria": "URL del servidor",
  "mcp.form.auth.placeholder": "Token de autorización (opcional)",
  "mcp.form.auth.keep": "Déjalo en blanco para conservar el token guardado",
  "mcp.form.auth.clear": "Eliminar la autorización guardada",
  "mcp.form.auth.originChanged":
    "El origen del endpoint cambió. Introduce una autorización nueva o elimina el valor guardado.",
  "mcp.form.auth.aria": "Token de autorización",
  "mcp.form.headers": "Cabeceras (objeto JSON)",
  "mcp.form.headers.placeholder": "X-API-Key=…",
  "mcp.form.headers.keep":
    "Déjalo en blanco para conservar las cabeceras guardadas",
  "mcp.form.headers.clear": "Eliminar las cabeceras guardadas",
  "mcp.form.headers.originChanged":
    "El origen del endpoint cambió. Introduce cabeceras nuevas o elimina los valores guardados.",
  "mcp.form.timeout": "Tiempo de espera (segundos) — opcional",
  "mcp.form.timeout.placeholder": "60",
  "mcp.form.timeout.invalid":
    "Introduce un número entero positivo de segundos o déjalo en blanco para no fijar un límite de tiempo.",
  "mcp.form.description.aria": "Descripción",
  "mcp.import": "Importar desde JSON",
  "mcp.import.confirm": "Importar",
  "mcp.importing": "Importando…",
  "mcp.import.ok_one": "{{count}} servidor importado",
  "mcp.import.ok_many": "{{count}} de servidores importados",
  "mcp.import.ok_other": "{{count}} servidores importados",
  "mcp.import.renamed":
    "Se renombraron para que el runtime las acepte: {{names}}",
  "mcp.import.interrupted":
    "La conexión de Runtime cambió: se importaron {{count}} de {{total}} servidores antes de la interrupción.",
  "mcp.import.partial": "Se importaron {{count}} de {{total}} servidores.",
  "mcp.import.error": "No se pudo importar esa configuración.",
  "mcp.import.invalidJson": "Introduce un JSON válido.",
  "mcp.import.missingTransport":
    "Indica un comando o una URL para el servidor «<server/>», pero no ambos.",
  "mcp.tools.title": "Herramientas de {{server}}",
  "mcp.tools.enabled": "Activada",
  "mcp.tools.autoApprove": "Aprobar automáticamente",
  "mcp.tools.empty": "Este servidor no expone herramientas",
  "mcp.tools.enable.aria": "Activar {{tool}}",
  "mcp.tools.autoApprove.aria": "Aprobar automáticamente {{tool}}",
  "mcp.error.save": "No se pudo guardar el servidor.",
  "mcp.error.remove": "No se pudo eliminar el servidor.",
  "mcp.error.toggle": "No se pudo modificar el servidor.",
  "mcp.error.signIn": "Error al iniciar sesión.",
  "mcp.error.reconnect": "No se pudo reconectar el servidor.",
  "mcp.error.test": "La prueba falló",
  "mcp.error.loadTools": "No se pudieron cargar las herramientas",
  "mcp.error.loadToolVisibility":
    "No se pudo cargar la visibilidad de las herramientas",
  "mcp.error.loadAutoApproveRules":
    "No se pudieron cargar las reglas de aprobación automática",
  "mcp.testOutcome.timedOut":
    "El servidor no respondió a tiempo: revisa el comando o la URL y vuelve a intentarlo.",
  "mcp.testOutcome.failed":
    "El servidor no se conectó o no ofreció una lista de herramientas válida. Revisa su configuración y vuelve a intentarlo.",
  "mcp.import.config": "Configuración JSON",
  "mcpStatus.mcp_authorization_required":
    "Este servidor requiere que inicies sesión antes de usarlo.",
  "mcpStatus.mcp_authorization_failed":
    "No se completó el inicio de sesión. Inténtalo de nuevo.",
  "mcpStatus.mcp_tool_discovery_failed":
    "Conectado, pero este servidor no devolvió una lista de herramientas utilizable. Revisa el servidor y vuelve a intentarlo.",
  "mcpStatus.mcp_configuration_failed":
    "No se pudo usar la configuración ni las credenciales guardadas de este servidor. Revisa sus ajustes y vuelve a intentarlo.",
  "mcpStatus.mcp_dial_failed":
    "No se pudo conectar con este servidor — revisa el comando o la URL y reinténtalo.",
  "common.cancel": "Cancelar",
  "common.close": "Cerrar",
  "command.uncertain":
    "El resultado del comando original no está confirmado. Concílialo antes de emitir otro comando o cambiar de catálogo.",
  "command.reconcile": "Conciliar el comando original",
  "read.retained": "Se conserva la última lectura aceptada.",
  "mcp.form.args.invalid":
    "Introduce una matriz JSON de cadenas, o déjalo en blanco si no hay argumentos.",
  "mcp.form.env.invalid":
    "Introduce un objeto JSON no vacío con valores de tipo cadena.",
  "mcp.form.headers.invalid":
    "Introduce un objeto JSON no vacío con valores de tipo cadena.",
  "mcp.import.invalidName":
    "El nombre del servidor «<server/>» no cumple las reglas de Runtime. Corrige la clave y vuelve a intentarlo.",
  "mcp.import.unsupportedTransport":
    "El «type» del servidor «<server/>» no está admitido. Usa «stdio» o «streamableHttp».",
  "mcp.import.headerConflict":
    "El servidor «<server/>» declara «<header/>» más de una vez. Conserva una sola declaración.",
};
