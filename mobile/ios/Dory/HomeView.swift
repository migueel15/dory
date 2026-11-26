import SwiftUI

struct RootView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        NavigationStack(path: $model.path) {
            HomeView()
                .navigationDestination(for: Route.self) { route in
                    switch route {
                    case .settings:
                        SettingsView()
                    case .node(let opened):
                        NodeScreen(opened: opened).id(opened.key)
                    }
                }
        }
        .fullScreenCover(isPresented: $model.capturing) {
            CaptureView()
        }
        .onChange(of: scenePhase, initial: true) { _, phase in
            model.phase(phase)
        }
    }
}

/// Paired environments, the ones available to pair and the way into each node.
struct HomeView: View {
    @EnvironmentObject private var model: AppModel
    @State private var pairing: PairTarget?
    @State private var forgetting: Node?

    var body: some View {
        List {
            Section("Entornos") {
                if model.nodes.isEmpty {
                    Text("Todavía no has emparejado ningún entorno.").foregroundStyle(.secondary)
                }
                ForEach(model.nodes) { node in
                    Button {
                        Task { await model.open(node) }
                    } label: {
                        NodeRow(name: node.name, detail: node.address, online: model.online[node.id] == true,
                                busy: model.opening == node.id)
                    }
                    .tint(.primary)
                    .disabled(model.opening != nil)
                    .swipeActions {
                        Button("Olvidar", role: .destructive) { forgetting = node }
                    }
                    .contextMenu {
                        Button("Olvidar", systemImage: "trash", role: .destructive) { forgetting = node }
                    }
                }
                if let error = model.openError {
                    Text(error).font(.footnote).foregroundStyle(.red)
                }
            }
            Section {
                if model.found.isEmpty {
                    Text("No hay entornos disponibles en este momento.").foregroundStyle(.secondary)
                }
                ForEach(model.found) { found in
                    Button {
                        pairing = .found(found)
                    } label: {
                        NodeRow(name: found.name, detail: "\(found.address) · \(found.via)", online: true, busy: false)
                    }
                    .tint(.primary)
                }
                Button {
                    pairing = .manual
                } label: {
                    Label("Emparejar con una dirección", systemImage: "plus")
                }
            } header: {
                Text("Disponibles para emparejar")
            } footer: {
                Text("Desliza hacia abajo para comprobar de nuevo los entornos. En un nodo abierto, desliza desde el borde izquierdo para volver aquí.")
            }
        }
        .navigationTitle("Dory")
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                NavigationLink(value: Route.settings) {
                    Image(systemName: "gearshape").accessibilityLabel("Ajustes")
                }
            }
        }
        .refreshable { await model.refresh() }
        .sheet(item: $pairing) { target in
            PairView(target: target).environmentObject(model)
        }
        .confirmationDialog(forgetting.map { "¿Olvidar \($0.name)?" } ?? "", isPresented: Binding(
            get: { forgetting != nil }, set: { if !$0 { forgetting = nil } }
        ), titleVisibility: .visible, presenting: forgetting) { node in
            Button("Olvidar", role: .destructive) { model.forget(node) }
        } message: { _ in
            Text("Tendrás que emparejarlo de nuevo.")
        }
    }
}

struct NodeRow: View {
    let name: String
    let detail: String
    let online: Bool
    let busy: Bool

    var body: some View {
        HStack(spacing: 12) {
            Circle()
                .fill(online ? Color.green : Color.secondary.opacity(0.5))
                .frame(width: 10, height: 10)
                .accessibilityLabel(online ? "Conectado" : "Sin conexión")
            VStack(alignment: .leading, spacing: 2) {
                Text(name).fontWeight(.semibold)
                Text(detail).font(.caption.monospaced()).foregroundStyle(.secondary).lineLimit(1)
            }
            Spacer()
            if busy { ProgressView() }
        }
        .contentShape(Rectangle())
    }
}

enum PairTarget: Identifiable {
    case manual
    case found(Found)

    var id: String {
        switch self {
        case .manual: return "manual"
        case .found(let found): return found.id
        }
    }
}

/// Pairs with the code shown on the computer under Environments.
struct PairView: View {
    let target: PairTarget
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var address = ""
    @State private var code = ""
    @State private var error: String?
    @State private var busy = false
    @FocusState private var focus: Field?

    private enum Field { case address, code }

    private var title: String {
        if case .found(let found) = target { return "Emparejar \(found.name)" }
        return "Emparejar entorno"
    }

    private var ready: Bool {
        !busy && !code.trimmingCharacters(in: .whitespaces).isEmpty
            && !address.trimmingCharacters(in: .whitespaces).isEmpty
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    if case .manual = target {
                        TextField("Dirección", text: $address, prompt: Text("192.168.1.20 o nombre del dispositivo"))
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .keyboardType(.URL)
                            .focused($focus, equals: .address)
                            .submitLabel(.next)
                            .onSubmit { focus = .code }
                    }
                    TextField("Código", text: $code, prompt: Text("1234-5678"))
                        .font(.body.monospaced())
                        .keyboardType(.numbersAndPunctuation)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .focused($focus, equals: .code)
                        .submitLabel(.go)
                        .onSubmit { if ready { Task { await pair() } } }
                } footer: {
                    Text("En el ordenador: Entornos → Mostrar código.")
                }
                if let error {
                    Section { Text(error).foregroundStyle(.red) }
                }
            }
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancelar") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if busy {
                        ProgressView()
                    } else {
                        Button("Emparejar") { Task { await pair() } }.disabled(!ready)
                    }
                }
            }
            .onAppear {
                if case .found(let found) = target {
                    address = found.address
                    focus = .code
                } else {
                    focus = .address
                }
            }
        }
        .presentationDetents([.medium, .large])
    }

    private func pair() async {
        busy = true
        error = nil
        defer { busy = false }
        do {
            try await model.pair(address: address, code: code)
            dismiss()
        } catch {
            self.error = error.localizedDescription
        }
    }
}
