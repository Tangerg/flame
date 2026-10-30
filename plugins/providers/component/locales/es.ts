export const es: Record<string, string> = {
  "providers.apiKey.replace": "Reemplazar clave de API…",
  "providers.apiKey.clear": "Quitar clave guardada",
  "providers.apiKey.clear.title": "¿Quitar la clave guardada?",
  "providers.apiKey.clear.body":
    "{{provider}} pierde su clave de API guardada. Tendrás que volver a pegarla para reconectar.",
  "providers.apiKey.placeholder": "Clave de API",
  "providers.apiKey.aria": "Clave de API de {{provider}}",
  "providers.baseUrl.placeholder": "URL base",
  "providers.baseUrl.aria": "URL base de {{provider}}",
  "providers.key": "clave {{masked}}",
  "providers.ready": "listo",
  "providers.notConfigured": "sin configurar",
  "providers.save": "Guardar",
  "providers.saving": "Guardando…",
  "providers.test": "Probar",
  "providers.testing": "Probando…",
  "providers.connectionOk": "Conexión correcta",
  "providers.error.save": "Error al guardar",
  "providers.error.test": "Error en la prueba",
  "providers.models.error": "No se pudieron cargar los modelos.",
  "providers.models.refresh": "Actualizar modelos",
  "providers.empty": "Sin proveedores",
  "providers.empty.sub":
    "El runtime no informa proveedores de LLM compatibles.",
  "providers.apiKey.envPlaceholder":
    "Usando la clave del entorno: escribe para sobrescribirla",
  "providers.fromEnv": "del entorno",
  "providers.utility.title": "Modelo auxiliar",
  "providers.utility.desc":
    "Modelo para resúmenes, títulos y extracción de hechos. Sin selección, usa el modelo predeterminado de Runtime. Los cambios se aplican a la siguiente llamada auxiliar, incluso durante una ejecución en curso.",
  "providers.utility.main": "Usar el modelo predeterminado de Runtime",
  "providers.utility.error": "No se pudo establecer el modelo auxiliar.",
  "providers.embedding.title": "Modelo de embeddings",
  "providers.embedding.desc":
    "Mejora opcionalmente la clasificación de Agent Memory. Si se desactiva, Memory sigue usando coincidencias por palabras clave. Se aplica a las búsquedas posteriores.",
  "providers.embedding.off": "Desactivado",
  "providers.embedding.none":
    "Todavía no hay ningún proveedor con embeddings configurado: añade uno arriba (por ejemplo OpenAI).",
  "providers.embedding.error": "No se pudo establecer el modelo de embeddings.",
  "providers.testOutcome.notConfigured":
    "Completa primero la configuración del proveedor.",
  "providers.testOutcome.failed":
    "No se pudo contactar al proveedor, o rechazó la petición de prueba.",
  "providers.apiKey.label": "Clave de API",
  "providers.baseUrl.label": "URL base",
  "providers.baseUrl.labelRequired": "URL base · obligatoria",
  "providers.baseUrl.required":
    "Este proveedor necesita una URL base para guardarse.",
  "providers.saveAndTest": "Guardar y probar",
  "providers.saved": "Guardado",
  "providers.role.loadError":
    "No se pudo cargar este ajuste, así que su valor actual es desconocido.",
  "providers.role.stale":
    "Se muestra el último valor conocido: la actualización falló.",
  "providers.setup.steps":
    "Elige un proveedor, pega su clave de API y pulsa «Guardar y probar». Es todo lo que necesitas para empezar.",
  "providers.setup.done":
    "Hay un proveedor listo. Cierra Ajustes para volver: tu borrador sigue ahí.",
  "providers.roles.heading": "Funciones de modelo",
  "rpcError.invalid_api_key":
    "El proveedor rechazó la API key — revísala en los ajustes del proveedor.",
  "rpcError.timeout":
    "La petición al proveedor del modelo agotó el tiempo — comprueba la conexión y reintenta.",
  title: "Proveedores",
  cancel: "Cancelar",
  reading: "Cargando",
  refresh: "Actualizar",
  retry: "Reintentar",
  "command.uncertain":
    "El resultado del comando original no está confirmado. Concílialo antes de emitir otro comando o cambiar de catálogo.",
  "command.reconcile": "Conciliar el comando original",
  "read.retained": "Se conserva la última lectura aceptada.",
  close: "Cerrar",
  "providers.embedding.noPreset":
    "Los proveedores configurados no ofrecen ningún modelo de embeddings predefinido.",
};
