import CryptoKit
import Foundation
import UIKit

/// An error with a message for the user, in Spanish like the rest of the app.
struct DoryError: LocalizedError, CustomLocalizedStringResourceConvertible {
    let message: String
    init(_ message: String) { self.message = message }
    var errorDescription: String? { message }
    var localizedStringResource: LocalizedStringResource { LocalizedStringResource(stringLiteral: message) }
}

/// Accepts a node only with the pinned key. With no pin it accepts any key
/// and records it: used only to pair, where the code proves who answered.
private final class PinnedDelegate: NSObject, URLSessionTaskDelegate {
    let want: String?
    let followRedirects: Bool
    private let lock = NSLock()
    private var got: String?

    init(want: String?, followRedirects: Bool) {
        self.want = want
        self.followRedirects = followRedirects
    }

    var seen: String? {
        lock.lock()
        defer { lock.unlock() }
        return got
    }

    func urlSession(_ session: URLSession, didReceive challenge: URLAuthenticationChallenge,
                    completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        guard challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust,
              let trust = challenge.protectionSpace.serverTrust, let pin = Pin.of(trust) else {
            completionHandler(.performDefaultHandling, nil)
            return
        }
        lock.lock()
        got = pin
        lock.unlock()
        // The pin, not the name, identifies the node.
        if let want, want != pin {
            completionHandler(.cancelAuthenticationChallenge, nil)
        } else {
            completionHandler(.useCredential, URLCredential(trust: trust))
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(followRedirects ? request : nil)
    }
}

/// Talks to the device API of a node over HTTPS with its pinned key.
enum NodeClient {
    struct Reply {
        let data: Data
        let response: HTTPURLResponse
        let pin: String?
    }

    static func send(_ address: String, _ path: String, method: String = "GET", pin: String?, token: String?,
                     body: Data? = nil, accept: String? = nil, followRedirects: Bool = true,
                     timeout: TimeInterval = 15) async throws -> Reply {
        guard let url = URL(string: "https://\(address)\(path)") else { throw DoryError("La dirección no es válida.") }
        var request = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: timeout)
        request.httpMethod = method
        if let token { request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        if let accept { request.setValue(accept, forHTTPHeaderField: "Accept") }
        if let body {
            request.httpBody = body
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let configuration = URLSessionConfiguration.ephemeral
        configuration.httpShouldSetCookies = false
        configuration.httpCookieAcceptPolicy = .never
        configuration.timeoutIntervalForRequest = timeout
        configuration.waitsForConnectivity = false
        let delegate = PinnedDelegate(want: pin, followRedirects: followRedirects)
        let session = URLSession(configuration: configuration, delegate: delegate, delegateQueue: nil)
        defer { session.finishTasksAndInvalidate() }
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            if let pin, let seen = delegate.seen, seen != pin {
                throw DoryError("La clave del nodo no es la emparejada.")
            }
            throw DoryError(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else { throw DoryError("El nodo no respondió por HTTPS.") }
        if http.statusCode == 401 || http.statusCode == 403 {
            throw DoryError(http.statusCode == 403 ? "Código incorrecto o caducado." : "El nodo ya no reconoce este móvil: empareja de nuevo.")
        }
        if http.statusCode >= 400 {
            let text = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
            throw DoryError("El nodo respondió \(http.statusCode): \(text)")
        }
        return Reply(data: data, response: http, pin: delegate.seen)
    }

    /// Pairs with a node using the code it shows in Environments.
    static func pair(address: String, code: String, name: String) async throws -> Node {
        let body = try JSONSerialization.data(withJSONObject: ["code": code, "name": name])
        let reply = try await send(address, "/device/v1/pair", method: "POST", pin: nil, token: nil, body: body)
        guard let pin = reply.pin,
              let out = try JSONSerialization.jsonObject(with: reply.data) as? [String: Any] else {
            throw DoryError("El nodo no devolvió un emparejamiento válido.")
        }
        guard (out["proof"] as? String) == proof(code: code, message: "\(pin)|device"), (out["pin"] as? String) == pin else {
            throw DoryError("El nodo no demostró conocer el código: puede haber alguien en medio.")
        }
        return Node(id: out["node_id"] as? String ?? "", name: out["name"] as? String ?? address,
                    address: address, pin: pin, token: out["token"] as? String ?? "")
    }

    /// Calls the API of a paired node with its token (e.g. /api/mesh).
    static func get(_ node: Node, _ path: String) async throws -> Data {
        try await send(node.address, path, pin: node.pin, token: node.token).data
    }

    static func json(_ node: Node, _ path: String) async throws -> Any {
        let data = try await get(node, path)
        return try JSONSerialization.jsonObject(with: data)
    }

    /// Trades the device token for the cookies of a web session.
    static func session(_ node: Node) async throws -> [HTTPCookie] {
        let reply = try await send(node.address, "/device/v1/session", method: "POST", pin: node.pin, token: node.token, body: Data())
        var headers: [String: String] = [:]
        for (key, value) in reply.response.allHeaderFields {
            if let key = key as? String, let value = value as? String { headers[key] = value }
        }
        guard let origin = URL(string: "https://\(node.address)/") else { return [] }
        return HTTPCookie.cookies(withResponseHeaderFields: headers, for: origin)
    }

    /// Saves an idea; the id makes a retry idempotent.
    static func sendIdea(_ node: Node, id: String, text: String) async throws {
        guard !node.pin.isEmpty, !node.token.isEmpty else { throw DoryError("Vuelve a emparejar el nodo de captura.") }
        let body = try JSONSerialization.data(withJSONObject: ["id": id, "text": text])
        let reply = try await send(node.address, "/api/ideas", method: "POST", pin: node.pin, token: node.token,
                                   body: body, followRedirects: false)
        let out = try? JSONSerialization.jsonObject(with: reply.data) as? [String: Any]
        guard (out?["id"] as? String) == id else { throw DoryError("El nodo no confirmó el guardado.") }
    }

    /// Same HMAC the node computes: hex(HMAC-SHA256(code, pin|device)).
    static func proof(code: String, message: String) -> String {
        let mac = HMAC<SHA256>.authenticationCode(for: Data(message.utf8), using: SymmetricKey(data: Data(code.utf8)))
        return mac.map { String(format: "%02x", $0) }.joined()
    }

    /// Name the node shows for this phone in Environments.
    @MainActor static var deviceName: String { UIDevice.current.name }
}
