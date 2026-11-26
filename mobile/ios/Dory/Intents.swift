import AppIntents

/// iOS has no replaceable digital assistant: Siri, the action button, Control
/// Center and Back Tap run these intents instead.
struct CaptureIdeaIntent: AppIntent {
    static var title: LocalizedStringResource = "Dictar idea"
    static var description = IntentDescription("Abre Dory para dictar una idea y guardarla en el nodo de captura.")
    static var openAppWhenRun = true

    @MainActor
    func perform() async throws -> some IntentResult {
        AppModel.shared.capturing = true
        return .result()
    }
}

/// Saves an idea that Siri asks for, without opening the app.
struct SaveIdeaIntent: AppIntent {
    static var title: LocalizedStringResource = "Guardar idea"
    static var description = IntentDescription("Guarda una idea en el nodo de captura de Dory sin abrir la app.")

    @Parameter(title: "Idea", requestValueDialog: "¿Qué idea quieres guardar?")
    var text: String

    func perform() async throws -> some IntentResult & ProvidesDialog {
        let idea = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !idea.isEmpty else { throw DoryError("La idea está vacía.") }
        let node = try AppSettings.captureDestination()
        try await NodeClient.sendIdea(node, id: UUID().uuidString, text: idea)
        return .result(dialog: "Idea guardada en \(node.name).")
    }
}

struct DoryShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(intent: CaptureIdeaIntent(),
                    phrases: ["Dictar idea en \(.applicationName)", "Nueva idea en \(.applicationName)"],
                    shortTitle: "Dictar idea", systemImageName: "mic")
        AppShortcut(intent: SaveIdeaIntent(),
                    phrases: ["Guardar idea en \(.applicationName)", "Apunta una idea en \(.applicationName)"],
                    shortTitle: "Guardar idea", systemImageName: "lightbulb")
    }
}
