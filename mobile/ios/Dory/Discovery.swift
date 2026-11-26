import Foundation
import Network

/// Finds nodes on the local network through the DNS-SD service each node
/// announces (_dory._tcp). iOS only lets apps with a special Apple permission
/// hear the UDP beacon Android uses, but Bonjour needs none.
@MainActor
final class Discovery {
    struct Service {
        let id: String
        let name: String
        var address: String?
        var resolvedAt: Date?
    }

    /// Called every few seconds for each node on the network: id, name, address.
    var onFound: (@MainActor (String, String, String) -> Void)?
    private var browser: NWBrowser?
    private var services: [NWEndpoint: Service] = [:]
    private var resolving: [NWEndpoint: NWConnection] = [:]
    private var timer: Timer?
    private var running = false

    func start() {
        running = true
        guard browser == nil else { return }
        let browser = NWBrowser(for: .bonjourWithTXTRecord(type: "_dory._tcp", domain: nil), using: .tcp)
        browser.browseResultsChangedHandler = { [weak self] results, _ in
            Task { @MainActor in self?.update(results) }
        }
        browser.stateUpdateHandler = { [weak self] state in
            guard case .failed = state else { return }
            Task { @MainActor in
                self?.browser?.cancel()
                self?.browser = nil
                try? await Task.sleep(nanoseconds: 5_000_000_000)
                if self?.running == true { self?.start() }
            }
        }
        browser.start(queue: .main)
        self.browser = browser
        timer?.invalidate()
        timer = Timer.scheduledTimer(withTimeInterval: 10, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.announce() }
        }
    }

    func stop() {
        running = false
        browser?.cancel()
        browser = nil
        timer?.invalidate()
        timer = nil
        resolving.values.forEach { $0.cancel() }
        resolving = [:]
        services = [:]
    }

    private func update(_ results: Set<NWBrowser.Result>) {
        var current: [NWEndpoint: Service] = [:]
        for result in results {
            guard case let .bonjour(record) = result.metadata, let id = record["id"], !id.isEmpty else { continue }
            let known = services[result.endpoint]
            current[result.endpoint] = Service(id: id, name: record["name"] ?? id,
                                               address: known?.address, resolvedAt: known?.resolvedAt)
        }
        for endpoint in services.keys where current[endpoint] == nil {
            resolving.removeValue(forKey: endpoint)?.cancel()
        }
        services = current
        announce()
    }

    /// Reports the nodes with a known address and refreshes stale addresses (DHCP).
    private func announce() {
        let now = Date()
        for (endpoint, service) in services {
            if let address = service.address { onFound?(service.id, service.name, address) }
            let stale = service.resolvedAt.map { now.timeIntervalSince($0) > 60 } ?? true
            if stale, resolving[endpoint] == nil { resolve(endpoint) }
        }
    }

    /// Resolves the IPv4 address and port of a service by opening a TCP connection.
    private func resolve(_ endpoint: NWEndpoint) {
        let parameters = NWParameters.tcp
        if let ip = parameters.defaultProtocolStack.internetProtocol as? NWProtocolIP.Options { ip.version = .v4 }
        let connection = NWConnection(to: endpoint, using: parameters)
        resolving[endpoint] = connection
        connection.stateUpdateHandler = { [weak self] state in
            switch state {
            case .ready:
                let remote = connection.currentPath?.remoteEndpoint
                Task { @MainActor in self?.resolved(endpoint, remote) }
            case .failed, .waiting:
                Task { @MainActor in self?.resolved(endpoint, nil) }
            default:
                break
            }
        }
        connection.start(queue: .main)
    }

    private func resolved(_ endpoint: NWEndpoint, _ remote: NWEndpoint?) {
        resolving.removeValue(forKey: endpoint)?.cancel()
        guard var service = services[endpoint] else { return }
        service.resolvedAt = Date()
        if case let .hostPort(host, port)? = remote, case let .ipv4(ip) = host {
            service.address = ip.rawValue.map { String($0) }.joined(separator: ".") + ":\(port.rawValue)"
        }
        services[endpoint] = service
        if let address = service.address { onFound?(service.id, service.name, address) }
    }
}
