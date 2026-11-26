import Foundation
import Security

/// A paired node: where it is, its pinned key and the device token.
struct Node: Codable, Identifiable, Hashable {
    var id: String
    var name: String
    var address: String
    var pin: String
    var token: String
}

/// Paired nodes live in the keychain, readable after the first unlock so the
/// background check of notices and Siri can use them.
enum NodeStore {
    private static let service = "net.dorlab.dory"
    private static let account = "nodes"

    private static var query: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: service,
         kSecAttrAccount as String: account]
    }

    static func load() -> [Node] {
        var request = query
        request[kSecReturnData as String] = true
        request[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        guard SecItemCopyMatching(request as CFDictionary, &item) == errSecSuccess, let data = item as? Data else { return [] }
        return (try? JSONDecoder().decode([Node].self, from: data)) ?? []
    }

    @discardableResult
    static func save(_ nodes: [Node]) -> Bool {
        guard let data = try? JSONEncoder().encode(nodes) else { return false }
        let attributes: [String: Any] = [kSecValueData as String: data,
                                         kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly]
        let status = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            return SecItemAdd(query.merging(attributes) { $1 } as CFDictionary, nil) == errSecSuccess
        }
        return status == errSecSuccess
    }
}

/// Settings of the phone, outside the keychain because they are not secret.
enum AppSettings {
    static let captureNodeKey = "capture.node"
    static let offlineKey = "capture.offline"
    static let noticesKey = "notices.enabled"

    static var captureNode: String {
        get { UserDefaults.standard.string(forKey: captureNodeKey) ?? "" }
        set { UserDefaults.standard.set(newValue, forKey: captureNodeKey) }
    }

    static var offlineSpeech: Bool { UserDefaults.standard.bool(forKey: offlineKey) }

    static var noticesEnabled: Bool { UserDefaults.standard.object(forKey: noticesKey) as? Bool ?? true }

    /// The node that receives dictated ideas: the only one, or the chosen one.
    static func captureDestination(retry: String? = nil) throws -> Node {
        let nodes = NodeStore.load()
        if nodes.isEmpty { throw DoryError("Abre Dory y comprueba que hay un nodo vinculado.") }
        if retry == nil, nodes.count == 1 { return nodes[0] }
        let id = retry ?? captureNode
        if !id.isEmpty, let node = nodes.first(where: { $0.id == id }) { return node }
        throw DoryError(retry == nil ? "Elige el nodo de captura en Dory." : "El nodo de esta idea ya no está vinculado.")
    }
}
