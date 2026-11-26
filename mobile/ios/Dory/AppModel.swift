import SwiftUI
import WebKit

/// A node that can be paired: heard on the local network or listed by the mesh of a paired one.
struct Found: Identifiable, Hashable {
    let id: String
    let name: String
    let address: String
    let via: String
    var expires: Date?
}

/// A node opened in the web view, on a given page.
struct OpenedNode: Hashable {
    let key = UUID()
    let node: Node
    let url: URL
}

enum Route: Hashable {
    case settings
    case node(OpenedNode)
}

/// State of the launcher: paired nodes, the ones available to pair and navigation.
@MainActor
final class AppModel: ObservableObject {
    static let shared = AppModel()

    @Published private(set) var nodes: [Node]
    @Published private(set) var online: [String: Bool] = [:]
    @Published private(set) var found: [Found] = []
    @Published private(set) var opening: String?
    @Published var path: [Route] = []
    @Published var openError: String?
    @Published var capturing = false
    @Published var captureNode: String {
        didSet { AppSettings.captureNode = captureNode }
    }

    private var local: [String: (found: Found, at: Date)] = [:]
    private var mesh: [String: (at: Date, peers: [Found])] = [:]
    private var checkVersions: [String: Int] = [:]
    private let discovery = Discovery()
    private var loop: Task<Void, Never>?

    private static let localTTL: TimeInterval = 30
    private static let meshTTL: TimeInterval = 60
    private static let discoveredTTL: TimeInterval = 75

    private init() {
        nodes = NodeStore.load()
        captureNode = AppSettings.captureNode
        Pin.trust(nodes)
        discovery.onFound = { [weak self] id, name, address in
            self?.heard(id: id, name: name, address: address)
        }
    }

    // MARK: Lifecycle

    func phase(_ phase: ScenePhase) {
        switch phase {
        case .active:
            discovery.start()
            startLoop()
            if !nodes.isEmpty { Notices.requestPermission() }
        case .background:
            discovery.stop()
            loop?.cancel()
            loop = nil
            Notices.schedule()
        default:
            break
        }
    }

    /// Checks the paired nodes every 30 s and expires stale entries every 5 s while on screen.
    private func startLoop() {
        guard loop == nil else { return }
        loop = Task { [weak self] in
            var tick = 0
            while !Task.isCancelled {
                guard let self else { return }
                if tick % 6 == 0 {
                    for node in self.nodes { Task { await self.check(node) } }
                }
                self.recompute()
                tick += 1
                try? await Task.sleep(nanoseconds: 5_000_000_000)
            }
        }
    }

    // MARK: Nodes

    private func store() {
        if !nodes.contains(where: { $0.id == captureNode }) { captureNode = "" }
        NodeStore.save(nodes)
        Pin.trust(nodes)
    }

    func pair(address: String, code: String) async throws {
        let node = try await NodeClient.pair(address: Address.withPort(address),
                                             code: code.trimmingCharacters(in: .whitespacesAndNewlines),
                                             name: NodeClient.deviceName)
        nodes = nodes.filter { $0.id != node.id } + [node]
        store()
        online[node.id] = true
        recompute()
        Notices.requestPermission()
        Task { await check(node) }
    }

    func forget(_ node: Node) {
        nodes.removeAll { $0.id == node.id }
        mesh[node.id] = nil
        online[node.id] = nil
        store()
        recompute()
    }

    func refresh() async {
        await withTaskGroup(of: Void.self) { group in
            for node in nodes {
                group.addTask { await self.check(node) }
            }
        }
        recompute()
    }

    /// Sees whether a paired node answers and learns the other nodes of its mesh,
    /// whose addresses also work from outside home through Tailscale.
    func check(_ node: Node) async {
        let version = (checkVersions[node.id] ?? 0) + 1
        checkVersions[node.id] = version
        do {
            let snapshot = try await NodeClient.json(node, "/api/mesh") as? [String: Any] ?? [:]
            guard checkVersions[node.id] == version, nodes.contains(where: { $0.id == node.id }) else { return }
            let via = "malla de \(node.name)"
            var peers: [Found] = []
            for peer in snapshot["peers"] as? [[String: Any]] ?? [] {
                guard peer["connected"] as? Bool == true, peer["revoked"] as? Bool != true,
                      let id = peer["id"] as? String, let raw = peer["address"] as? String,
                      let address = Address.meshAddress(raw) else { continue }
                peers.append(Found(id: id, name: peer["name"] as? String ?? id, address: address, via: via))
            }
            for item in snapshot["discovered"] as? [[String: Any]] ?? [] {
                guard let id = item["id"] as? String, !id.isEmpty, let raw = item["address"] as? String,
                      let address = Address.meshAddress(raw), let seen = Dates.parse(item["seen"] as? String),
                      Date().timeIntervalSince(seen) <= Self.discoveredTTL else { continue }
                peers.append(Found(id: id, name: item["name"] as? String ?? id, address: address, via: via,
                                   expires: seen.addingTimeInterval(Self.discoveredTTL)))
            }
            online[node.id] = true
            mesh[node.id] = (Date(), peers)
        } catch {
            guard checkVersions[node.id] == version, nodes.contains(where: { $0.id == node.id }) else { return }
            online[node.id] = false
            mesh[node.id] = nil
        }
        recompute()
    }

    private func heard(id: String, name: String, address: String) {
        if let index = nodes.firstIndex(where: { $0.id == id }) {
            // A paired node changed its address (DHCP): follow it.
            if nodes[index].address != address {
                nodes[index].address = address
                store()
                let node = nodes[index]
                Task { await check(node) }
            } else if online[id] != true {
                let node = nodes[index]
                Task { await check(node) }
            }
            return
        }
        local[id] = (Found(id: id, name: name, address: address, via: "red local"), Date())
        recompute()
    }

    private func recompute() {
        let now = Date()
        var all: [String: Found] = [:]
        for source in mesh.values where now.timeIntervalSince(source.at) <= Self.meshTTL {
            for peer in source.peers where peer.expires.map({ now <= $0 }) ?? true { all[peer.id] = peer }
        }
        for (id, entry) in local {
            if now.timeIntervalSince(entry.at) <= Self.localTTL { all[id] = entry.found } else { local[id] = nil }
        }
        let saved = Set(nodes.map(\.id))
        let pending = all.values.filter { !saved.contains($0.id) }
            .sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
        if pending != found { found = pending }
    }

    // MARK: Opening

    /// Trades the token for a web session and shows the node in the web view.
    func open(_ node: Node, path page: String = "/") async {
        guard opening == nil else { return }
        openError = nil
        opening = node.id
        defer { opening = nil }
        do {
            let cookies = try await NodeClient.session(node)
            let cookieStore = WKWebsiteDataStore.default().httpCookieStore
            for cookie in cookies { await cookieStore.setCookie(cookie) }
            let target = page.hasPrefix("/") ? page : "/"
            guard let url = URL(string: "https://\(node.address)\(target)") else { throw DoryError("La dirección no es válida.") }
            path = [.node(OpenedNode(node: node, url: url))]
        } catch {
            openError = "\(node.name): \(error.localizedDescription)"
        }
    }

    /// A tapped notice opens its node, with a fresh session, on the page it points to.
    func openNotice(node id: String, url: String) async {
        guard let node = nodes.first(where: { $0.id == id }) else { return }
        capturing = false
        await open(node, path: url)
    }
}

enum Dates {
    /// Parses the RFC 3339 times of the node, with or without fractions of a second.
    static func parse(_ text: String?) -> Date? {
        guard let text else { return nil }
        let whole = text.replacingOccurrences(of: #"\.\d+"#, with: "", options: .regularExpression)
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter.date(from: whole)
    }
}
