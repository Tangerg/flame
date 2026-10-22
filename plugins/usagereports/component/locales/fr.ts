export const fr: Record<string, string> = {
  "panel.title": "Rapports d’utilisation enregistrés",
  "panel.description":
    "Copies immuables des rapports de comptabilité principale. La comptabilité en direct n’est pas affectée.",
  "period.daily.label": "Quotidien",
  "period.daily.window": "dernières 24 heures",
  "period.weekly.label": "Hebdomadaire",
  "period.weekly.window": "7 derniers jours",
  "report.title": "Rapport {{period}} · {{observed}}",
  "row.title": "{{period}} · {{observed}}",
  "period.windowed": "{{label}} · {{window}}",
  "row.description": "{{window}} · Début {{since}}",
  "report.scope.label": "Rapport enregistré",
  "report.refresh": "Actualiser le rapport",
  "report.back": "Retour aux archives",
  "archive.actions.label": "Actions des archives",
  "capture.period.label": "Période de capture",
  "capture.save": "Enregistrer le rapport",
  "archive.refresh": "Actualiser les archives",
  "evidence.beginning": "Début des preuves conservées",
  "cost.notReported": "Non signalé",
  "pending.label": "Commande d’archive d’origine",
  "pending.note": "Confirmation en attente pour la commande d’origine.",
  "pending.check": "Vérifier le résultat d’origine",
  "pending.retire": "Retirer la commande d’origine",
  "archive.title": "Archives",
  "archive.count":
    "{{count}} rapports enregistrés. La capacité est de 64 ; supprimez un rapport pour libérer de la place.",
  "archive.capture.note":
    "Une capture copie la comptabilité principale des {{daily}} (quotidien) ou des {{weekly}} (hebdomadaire) avant son exécution, et non un jour ou une semaine calendaires.",
  "archive.reading": "Lecture des rapports enregistrés…",
  "archive.error": "Les archives n’ont pas pu être lues : {{error}}.",
  "archive.error.retained":
    "Les archives n’ont pas pu être lues : {{error}}. Les dernières archives acceptées restent affichées.",
  "archive.empty.title": "Aucun rapport enregistré",
  "archive.empty.sub":
    "Choisissez une période et enregistrez le rapport principal actuel.",
  "archive.list.label": "Rapports enregistrés",
  "report.view": "Voir le rapport",
  "report.view.aria": "Voir le rapport : {{title}}",
  "removal.title": "Supprimer {{title}} ?",
  "removal.description":
    "Seule cette copie enregistrée est supprimée. Les preuves et les totaux de la comptabilité principale restent inchangés, et la copie ne peut pas être restaurée.",
  "removal.close": "Fermer la confirmation de suppression",
  "removal.cancel": "Annuler",
  "report.remove": "Supprimer le rapport enregistré",
  "removal.removing": "Suppression du rapport…",
  "fact.period": "Période",
  "fact.observation": "Observation principale",
  "fact.lowerBound": "Limite inférieure réelle",
  "report.saved.note":
    "Copie enregistrée d’un rapport de comptabilité principale. Elle ne se met pas à jour lorsque la comptabilité en direct change.",
  "report.reading": "Lecture du rapport enregistré…",
  "report.error.refresh":
    "Le rapport enregistré n’a pas pu être actualisé : {{error}}. Le dernier rapport accepté reste visible.",
  "report.error.read": "Le rapport enregistré n’a pas pu être lu : {{error}}.",
  "report.totals.title": "Totaux principaux enregistrés",
  "report.totals.description":
    "Les valeurs correspondent à la réponse principale enregistrée. L’utilisation et les prix inconnus conservent leur signification principale.",
  "fact.attempts": "Tentatives de modèle",
  "fact.inputTokens": "Tokens d’entrée connus",
  "fact.outputTokens": "Tokens de sortie connus",
  "fact.knownSubtotal": "Sous-total du prix connu (USD)",
  "fact.totalCost": "Coût total (USD)",
  "fact.usageRecorded": "Utilisation enregistrée",
  "fact.pricedAttempts": "Tentatives tarifées",
  "fact.outcomeUnknown": "Résultat inconnu",
  "report.breakdown.title": "Détail principal quotidien",
  "report.breakdown.row": "{{attempts}} tentatives · {{cost}} USD connus",
  "report.identity.title": "Identité exacte du rapport",
  "fact.reportId": "ID du rapport",
  "fact.observedAt": "Observé à (UTC)",
  "fact.lowerBoundUtc": "Limite inférieure (UTC)",
  "report.actions.label": "Actions du rapport enregistré",
  "message.saved": "Rapport principal enregistré.",
  "message.savedArchived":
    "Rapport principal enregistré : {{title}}. Il figure dans les archives.",
  "message.removed":
    "Rapport enregistré supprimé. La comptabilité principale est inchangée.",
  "message.removalConfirmed":
    "Suppression du rapport d’origine confirmée. La comptabilité principale est inchangée.",
  "message.unknown":
    "La confirmation est inconnue. Vérifiez le résultat d’origine avant de soumettre une autre modification des archives.",
  "message.archiveFailed": "La modification des archives a échoué : {{error}}",
  "message.captureRejected": "La capture n’a pas pu être admise : {{error}}",
  "message.removalRejected":
    "La suppression n’a pas pu être admise : {{error}}",
  "message.retired":
    "Commande d’origine retirée. Tout rapport déjà enregistré reste dans les archives.",
  "message.retireFailed":
    "La commande d’origine n’a pas pu être retirée : {{error}}",
};
