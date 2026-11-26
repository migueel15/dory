import AppIntents
import Speech
import SwiftUI
import UserNotifications

struct SettingsView: View {
    @EnvironmentObject private var model: AppModel
    @AppStorage(AppSettings.offlineKey) private var offline = false
    @AppStorage(AppSettings.noticesKey) private var notices = true
    @State private var permission: UNAuthorizationStatus = .notDetermined
    @State private var latest: (tag: String, build: Int)?
    @State private var versionError = false
    @Environment(\.openURL) private var openURL

    private let onDevice = SFSpeechRecognizer()?.supportsOnDeviceRecognition ?? false
    private let build = Int(Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "") ?? 0

    var body: some View {
        Form {
            Section {
                Button {
                    model.capturing = true
                } label: {
                    Label("Dictar una idea", systemImage: "mic")
                }
                if model.nodes.count >= 2 {
                    Picker("Nodo de captura", selection: $model.captureNode) {
                        Text("Elige el nodo").tag("")
                        ForEach(model.nodes) { node in
                            Text(node.name.isEmpty ? node.address : node.name).tag(node.id)
                        }
                    }
                }
                if onDevice {
                    Toggle("Reconocer sin conexión", isOn: $offline)
                }
                ShortcutsLink()
                    .shortcutsLinkStyle(.automaticOutline)
            } header: {
                Text("Dictado de ideas")
            } footer: {
                Text("Di «Oye Siri, dictar idea en Dory» o asigna el atajo «Dictar idea» al botón de acción, al centro de control o a Tocar atrás. «Guardar idea» la guarda sin abrir la app.")
            }

            Section {
                Toggle("Avisos con la app cerrada", isOn: $notices)
                    .onChange(of: notices) { _, enabled in
                        if enabled {
                            Notices.requestPermission()
                        }
                        Notices.schedule()
                        Task { await readPermission() }
                    }
                if notices && permission == .denied {
                    Button("Permitir notificaciones en Ajustes") { openSettings() }
                }
            } header: {
                Text("Avisos")
            } footer: {
                Text("Sin cuenta de pago de Apple no hay push inmediato: iOS deja a Dory comprobar los avisos de la malla cada cierto tiempo. Con la app abierta, los avisos salen en la interfaz del nodo.")
            }

            Section {
                Button("Micrófono, cámara y red local") { openSettings() }
            } header: {
                Text("Permisos")
            } footer: {
                Text("Se gestionan en los ajustes de iOS para Dory.")
            }

            Section("Versión") {
                LabeledContent("Instalada", value: "ios-\(build)")
                if let latest {
                    if latest.build > build {
                        Link("Disponible: \(latest.tag)", destination: URL(string: "https://github.com/migueel15/dory/releases/tag/\(latest.tag)")!)
                    } else {
                        Text("Estás en la última").foregroundStyle(.secondary)
                    }
                } else if versionError {
                    Text("No se pudo consultar GitHub").foregroundStyle(.secondary)
                }
            }
        }
        .navigationTitle("Ajustes")
        .task {
            await readPermission()
            await readLatest()
        }
    }

    private func openSettings() {
        if let url = URL(string: UIApplication.openSettingsURLString) { openURL(url) }
    }

    private func readPermission() async {
        permission = await UNUserNotificationCenter.current().notificationSettings().authorizationStatus
    }

    private func readLatest() async {
        do {
            let url = URL(string: "https://api.github.com/repos/migueel15/dory/releases?per_page=50")!
            let (data, _) = try await URLSession.shared.data(from: url)
            let releases = try JSONSerialization.jsonObject(with: data) as? [[String: Any]] ?? []
            for release in releases {
                guard let tag = release["tag_name"] as? String, tag.hasPrefix("ios-") else { continue }
                let parts = tag.split(separator: "-")
                guard parts.count >= 3, let number = Int(parts[1]) else { continue }
                latest = (tag, number)
                return
            }
        } catch {
            versionError = true
        }
    }
}
