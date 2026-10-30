export const fr: Record<string, string> = {
  "mcp.error.toolPolicy": "Impossible de modifier la politique de l’outil.",
  "mcp.tools.policyHint":
    "Enregistrement immédiat. La visibilité des outils s’applique aux nouvelles exécutions. L’approbation automatique définit une règle globale d’autorisation ; les règles plus précises restent applicables.",
  "mcp.tools.nameConflict":
    "Masqué car {{name}} entre en conflit avec {{sources}}.",
  "mcp.add": "Ajouter un serveur",
  "mcp.save": "Enregistrer",
  "mcp.saving": "Enregistrement…",
  "mcp.delete": "Supprimer",
  "mcp.delete.title": "Supprimer ce serveur ?",
  "mcp.delete.body":
    "« {{server}} » et ses outils sont supprimés. Vous pourrez l’ajouter de nouveau plus tard.",
  "mcp.test": "Tester",
  "mcp.testing": "Test en cours…",
  "mcp.connectionOk": "Connexion OK",
  "mcp.toolCount_one": "{{count}} outil",
  "mcp.toolCount_many": "{{count}} d’outils",
  "mcp.toolCount_other": "{{count}} outils",
  "mcp.empty": "Aucun serveur MCP",
  "mcp.enable.aria": "Activer {{server}}",
  "mcp.edit": "Modifier {{server}}",
  "mcp.transport.stdio": "stdio",
  "mcp.transport.http": "Streamable HTTP",
  "mcp.signIn": "Se connecter",
  "mcp.signingIn": "Connexion…",
  "mcp.signIn.aria": "Se connecter à {{server}}",
  "mcp.signingIn.aria": "Connexion à {{server}}…",
  "mcp.reconnect": "Reconnecter",
  "mcp.reconnect.aria": "Reconnecter {{server}}",
  "mcp.status.disabled": "Désactivé",
  "mcp.status.disconnected": "Déconnecté",
  "mcp.status.connecting": "Connexion en cours…",
  "mcp.status.connected": "Connecté",
  "mcp.status.failed": "Échec de la connexion",
  "mcp.status.needsAuth": "Connexion requise",
  "mcp.form.name.aria": "Nom du serveur",
  "mcp.form.transport.aria": "Transport",
  "mcp.form.command.placeholder": "npx",
  "mcp.form.command.aria": "Commande",
  "mcp.form.args": "Arguments — un par ligne",
  "mcp.form.args.placeholder":
    "-y\n@modelcontextprotocol/server-filesystem\n/chemin",
  "mcp.form.env": "Environnement — CLÉ=valeur, un par ligne",
  "mcp.form.env.placeholder": "API_KEY=…",
  "mcp.form.env.keep": "Laissez vide pour conserver l’environnement enregistré",
  "mcp.form.env.clear": "Supprimer l’environnement enregistré",
  "mcp.form.env.targetChanged":
    "La cible du processus a changé. Saisissez un nouvel environnement ou supprimez les valeurs enregistrées.",
  "mcp.form.dir.aria": "Répertoire de travail",
  "mcp.form.url.placeholder": "https://example.com/mcp",
  "mcp.form.url.aria": "URL du serveur",
  "mcp.form.auth.placeholder": "Jeton d'autorisation (facultatif)",
  "mcp.form.auth.keep": "Laissez vide pour conserver le jeton enregistré",
  "mcp.form.auth.clear": "Supprimer l’autorisation enregistrée",
  "mcp.form.auth.originChanged":
    "L’origine du point de terminaison a changé. Saisissez une nouvelle autorisation ou supprimez la valeur enregistrée.",
  "mcp.form.auth.aria": "Jeton d'autorisation",
  "mcp.form.headers": "En-têtes — CLÉ=valeur, un par ligne",
  "mcp.form.headers.placeholder": "X-API-Key=…",
  "mcp.form.headers.keep":
    "Laissez vide pour conserver les en-têtes enregistrés",
  "mcp.form.headers.clear": "Supprimer les en-têtes enregistrés",
  "mcp.form.headers.originChanged":
    "L’origine du point de terminaison a changé. Saisissez de nouveaux en-têtes ou supprimez les valeurs enregistrées.",
  "mcp.form.timeout": "Délai d'expiration (secondes) — facultatif",
  "mcp.form.timeout.placeholder": "60",
  "mcp.form.timeout.invalid":
    "Saisissez un nombre entier positif de secondes, ou laissez vide pour ne fixer aucune limite de temps.",
  "mcp.form.description.aria": "Description",
  "mcp.import": "Importer depuis JSON",
  "mcp.import.confirm": "Importer",
  "mcp.importing": "Import en cours…",
  "mcp.import.ok_one": "{{count}} serveur importé",
  "mcp.import.ok_many": "{{count}} de serveurs importés",
  "mcp.import.ok_other": "{{count}} serveurs importés",
  "mcp.import.renamed": "Renommés pour que le runtime les accepte : {{names}}",
  "mcp.import.interrupted":
    "Connexion Runtime modifiée : {{count}} serveurs sur {{total}} importés avant l’interruption.",
  "mcp.import.partial": "{{count}} serveurs sur {{total}} importés.",
  "mcp.import.error": "Impossible d'importer cette configuration.",
  "mcp.import.nameCollision":
    "Les serveurs « {{first}} » et « {{second}} » deviennent tous deux « {{canonical}} » ; donnez-leur des noms distincts",
  "mcp.import.invalidJson": "Saisissez un JSON valide.",
  "mcp.import.invalidShape":
    "Utilisez une configuration MCP valide avec un objet « mcpServers ».",
  "mcp.import.invalidName":
    "Donnez au serveur « {{server}} » un nom contenant a–z ou 0–9.",
  "mcp.import.missingTransport":
    "Pour le serveur « {{server}} », utilisez stdio avec une commande ou Streamable HTTP avec une URL.",
  "mcp.import.missingCommand":
    "Ajoutez une commande pour le serveur « {{server}} ».",
  "mcp.import.missingUrl": "Ajoutez une URL pour le serveur « {{server}} ».",
  "mcp.import.invalidTimeout":
    "Définissez un délai d’expiration en secondes entières positives pour le serveur « {{server}} », ou omettez-le pour ne fixer aucune limite de temps.",
  "mcp.import.empty": "Ajoutez au moins un serveur sous « mcpServers ».",
  "mcp.tools.title": "Outils de {{server}}",
  "mcp.tools.enabled": "Activé",
  "mcp.tools.autoApprove": "Approbation automatique",
  "mcp.tools.empty": "Ce serveur n'expose aucun outil",
  "mcp.tools.enable.aria": "Activer {{tool}}",
  "mcp.tools.autoApprove.aria": "Approuver automatiquement {{tool}}",
  "mcp.error.save": "Impossible d'enregistrer le serveur.",
  "mcp.error.remove": "Impossible de supprimer le serveur.",
  "mcp.error.toggle": "Impossible de modifier le serveur.",
  "mcp.error.signIn": "Échec de la connexion.",
  "mcp.error.reconnect": "Impossible de reconnecter le serveur.",
  "mcp.error.test": "Échec du test",
  "mcp.error.loadTools": "Impossible de charger les outils",
  "mcp.error.loadToolVisibility":
    "Impossible de charger la visibilité des outils",
  "mcp.error.loadAutoApproveRules":
    "Impossible de charger les règles d'approbation automatique",
  "mcp.testOutcome.timedOut":
    "Le serveur n'a pas répondu à temps — vérifiez la commande ou l'URL et réessayez.",
  "mcp.testOutcome.failed":
    "Le serveur ne s’est pas connecté ou n’a pas fourni de liste d’outils valide. Vérifiez ses réglages et réessayez.",
  "mcp.import.config": "Configuration JSON",
  "mcpStatus.mcp_authorization_required":
    "Ce serveur demande une connexion avant de pouvoir servir.",
  "mcpStatus.mcp_authorization_failed":
    "La connexion n'a pas abouti. Réessaie.",
  "mcpStatus.mcp_tool_discovery_failed":
    "Connecté, mais ce serveur n'a pas renvoyé de liste d'outils utilisable. Vérifiez le serveur et réessayez.",
  "mcpStatus.mcp_configuration_failed":
    "La configuration ou les identifiants enregistrés de ce serveur n'ont pas pu être utilisés. Vérifiez ses réglages et réessayez.",
  "mcpStatus.mcp_dial_failed":
    "Impossible de joindre ce serveur — vérifie la commande ou l'URL et réessaie.",
  "common.cancel": "Annuler",
  "common.close": "Fermer",
  title: "Serveurs MCP",
  "command.uncertain":
    "Le résultat de la commande d’origine n’est pas confirmé. Rapprochez-le avant d’émettre une autre commande ou de changer de catalogue.",
  "command.reconcile": "Rapprocher la commande d’origine",
  "read.retained": "La dernière lecture acceptée est conservée.",
  reading: "Chargement",
  refresh: "Actualiser",
  retry: "Réessayer",
};
