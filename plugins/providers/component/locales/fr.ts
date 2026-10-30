export const fr: Record<string, string> = {
  "providers.apiKey.replace": "Remplacer la clé API…",
  "providers.apiKey.clear": "Supprimer la clé enregistrée",
  "providers.apiKey.clear.title": "Supprimer la clé enregistrée ?",
  "providers.apiKey.clear.body":
    "{{provider}} perd sa clé API enregistrée. Vous devrez la recoller pour vous reconnecter.",
  "providers.apiKey.placeholder": "Clé API",
  "providers.apiKey.aria": "Clé API {{provider}}",
  "providers.baseUrl.placeholder": "URL de base",
  "providers.baseUrl.aria": "URL de base {{provider}}",
  "providers.key": "clé {{masked}}",
  "providers.ready": "prêt",
  "providers.notConfigured": "non configuré",
  "providers.save": "Enregistrer",
  "providers.saving": "Enregistrement…",
  "providers.test": "Tester",
  "providers.testing": "Test…",
  "providers.connectionOk": "Connexion OK",
  "providers.error.save": "Échec de l'enregistrement",
  "providers.error.test": "Échec du test",
  "providers.models.error": "Impossible de charger les modèles.",
  "providers.models.refresh": "Actualiser les modèles",
  "providers.empty": "Aucun fournisseur",
  "providers.empty.sub":
    "Le runtime ne signale aucun fournisseur LLM pris en charge.",
  "providers.apiKey.envPlaceholder":
    "Clé d'environnement utilisée — saisissez pour la remplacer",
  "providers.fromEnv": "depuis l'environnement",
  "providers.utility.title": "Modèle utilitaire",
  "providers.utility.desc":
    "Modèle pour les résumés, titres et extractions de faits. Sans sélection, le modèle par défaut de Runtime est utilisé. Les changements prennent effet au prochain appel auxiliaire, même pendant une exécution en cours.",
  "providers.utility.main": "Utiliser le modèle par défaut de Runtime",
  "providers.utility.error": "Impossible de définir le modèle utilitaire.",
  "providers.embedding.title": "Modèle d'embedding",
  "providers.embedding.desc":
    "Améliore facultativement le classement d'Agent Memory. Désactivé, Memory continue avec la recherche par mots-clés. S’applique aux recherches suivantes.",
  "providers.embedding.off": "Désactivé",
  "providers.embedding.none":
    "Aucun fournisseur compatible embedding pour l'instant — ajoutez-en un ci-dessus (par ex. OpenAI).",
  "providers.embedding.error": "Impossible de définir le modèle d'embedding.",
  "providers.testOutcome.notConfigured":
    "Terminez d'abord la configuration du fournisseur.",
  "providers.testOutcome.failed":
    "Le fournisseur était injoignable, ou il a refusé la requête de test.",
  "providers.apiKey.label": "Clé API",
  "providers.baseUrl.label": "URL de base",
  "providers.baseUrl.labelRequired": "URL de base · requise",
  "providers.baseUrl.required":
    "Ce fournisseur a besoin d’une URL de base pour être enregistré.",
  "providers.saveAndTest": "Enregistrer et tester",
  "providers.saved": "Enregistré",
  "providers.role.loadError":
    "Impossible de charger ce réglage : sa valeur actuelle est inconnue.",
  "providers.role.stale":
    "Dernière valeur connue affichée — l’actualisation a échoué.",
  "providers.setup.steps":
    "Choisissez un fournisseur, collez sa clé API puis « Enregistrer et tester ». C’est tout ce qu’il faut pour commencer.",
  "providers.setup.done":
    "Un fournisseur est prêt. Fermez les paramètres pour revenir : votre brouillon est toujours là.",
  "providers.roles.heading": "Rôles des modèles",
  "rpcError.invalid_api_key":
    "Le fournisseur a rejeté la clé API — vérifie-la dans les réglages du fournisseur.",
  "rpcError.timeout":
    "La requête au fournisseur de modèle a expiré — vérifie ta connexion et réessaie.",
  title: "Fournisseurs",
  cancel: "Annuler",
  reading: "Chargement",
  refresh: "Actualiser",
  retry: "Réessayer",
  "command.uncertain":
    "Le résultat de la commande d’origine n’est pas confirmé. Rapprochez-le avant d’émettre une autre commande ou de changer de catalogue.",
  "command.reconcile": "Rapprocher la commande d’origine",
  "read.retained": "La dernière lecture acceptée est conservée.",
  close: "Fermer",
  "providers.embedding.noPreset":
    "Aucun modèle de plongement prédéfini n’est disponible pour les fournisseurs configurés.",
};
