import CryptoKit
import Foundation
import Security

/// Nodes use self-signed certificates. Trust comes from pairing: the app pins
/// the SHA-256 of the public key info of each node, per host, like Android.
enum Pin {
    private static let lock = NSLock()
    private static var hosts: [String: String] = [:]

    /// Trusts exactly the paired nodes: host:port → pin.
    static func trust(_ nodes: [Node]) {
        var next: [String: String] = [:]
        for node in nodes {
            guard let (host, port) = Address.split(node.address) else { continue }
            next[key(host, port)] = node.pin
        }
        lock.lock()
        hosts = next
        lock.unlock()
    }

    static func pinned(host: String, port: Int) -> String? {
        lock.lock()
        defer { lock.unlock() }
        return hosts[key(host, port)]
    }

    /// Whether a URL belongs to a paired node.
    static func pinned(_ url: URL) -> String? {
        guard url.scheme?.lowercased() == "https", let host = url.host else { return nil }
        return pinned(host: host, port: url.port ?? 443)
    }

    private static func key(_ host: String, _ port: Int) -> String {
        let bare = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        return "\(bare):\(port)"
    }

    static func leaf(_ trust: SecTrust) -> SecCertificate? {
        (SecTrustCopyCertificateChain(trust) as? [SecCertificate])?.first
    }

    /// Pin of the certificate the server presented in this trust.
    static func of(_ trust: SecTrust?) -> String? {
        guard let trust, let leaf = leaf(trust) else { return nil }
        return of(leaf)
    }

    /// Same value the node and Android compute: base64url(SHA-256(SubjectPublicKeyInfo)).
    static func of(_ certificate: SecCertificate) -> String? {
        let der = [UInt8](SecCertificateCopyData(certificate) as Data)
        guard let info = subjectPublicKeyInfo(der) else { return nil }
        return Data(SHA256.hash(data: info)).base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    /// Reads one DER element at offset i: its tag and where its content starts and ends.
    private static func element(_ d: [UInt8], _ i: Int) -> (tag: UInt8, start: Int, end: Int)? {
        guard i >= 0, i + 2 <= d.count else { return nil }
        var length = Int(d[i + 1])
        var p = i + 2
        if length & 0x80 != 0 {
            let count = length & 0x7f
            guard count >= 1, count <= 4, p + count <= d.count else { return nil }
            length = 0
            for _ in 0..<count {
                length = length << 8 | Int(d[p])
                p += 1
            }
        }
        guard p + length <= d.count else { return nil }
        return (d[i], p, p + length)
    }

    /// Extracts the SubjectPublicKeyInfo of an X.509 certificate, header included.
    static func subjectPublicKeyInfo(_ der: [UInt8]) -> [UInt8]? {
        guard let certificate = element(der, 0), certificate.tag == 0x30,
              let tbs = element(der, certificate.start), tbs.tag == 0x30 else { return nil }
        var i = tbs.start
        if let version = element(der, i), version.tag == 0xa0 { i = version.end }
        // serialNumber, signature, issuer, validity and subject come before the key.
        for _ in 0..<5 {
            guard let skipped = element(der, i), skipped.end <= tbs.end else { return nil }
            i = skipped.end
        }
        guard let key = element(der, i), key.tag == 0x30, key.end <= tbs.end else { return nil }
        return Array(der[i..<key.end])
    }
}

/// Addresses are host:port, with IPv6 hosts in brackets.
enum Address {
    static let port = 7710

    static func split(_ address: String) -> (String, Int)? {
        if address.hasPrefix("["), let close = address.firstIndex(of: "]") {
            let host = String(address[address.index(after: address.startIndex)..<close])
            let rest = address[address.index(after: close)...]
            if rest.hasPrefix(":"), let value = Int(rest.dropFirst()) { return (host, value) }
            return (host, Self.port)
        }
        let parts = address.split(separator: ":", omittingEmptySubsequences: false)
        if parts.count == 2, let value = Int(parts[1]) { return (String(parts[0]), value) }
        if parts.count == 1, !address.isEmpty { return (address, Self.port) }
        return nil
    }

    /// Adds the mesh port when the address has none.
    static func withPort(_ address: String) -> String {
        let trimmed = address.trimmingCharacters(in: .whitespacesAndNewlines)
        let colons = trimmed.filter { $0 == ":" }.count
        let hasPort = trimmed.range(of: #":\d+$"#, options: .regularExpression) != nil
        if trimmed.hasPrefix("[") { return hasPort && trimmed.contains("]:") ? trimmed : "\(trimmed):\(Self.port)" }
        if colons > 1 { return "[\(trimmed)]:\(Self.port)" }
        return hasPort ? trimmed : "\(trimmed):\(Self.port)"
    }

    /// The host of an address with the mesh port: peers report their own port, the phone uses 7710.
    static func meshAddress(_ address: String) -> String? {
        guard let (host, _) = split(address), !host.isEmpty else { return nil }
        return host.contains(":") ? "[\(host)]:\(port)" : "\(host):\(port)"
    }
}
