import SwiftUI
import WebKit

/// The interface of a paired node, full screen, as on any phone.
struct NodeScreen: View {
    let opened: OpenedNode
    @Environment(\.dismiss) private var dismiss
    @StateObject private var page: NodePage

    init(opened: OpenedNode) {
        self.opened = opened
        _page = StateObject(wrappedValue: NodePage(opened: opened))
    }

    var body: some View {
        ZStack {
            // Fills the status bar and home indicator areas with the page's own background.
            Color(uiColor: page.background).ignoresSafeArea()
            NodeContainer(page: page).ignoresSafeArea()
            if let failure = page.failure {
                VStack(spacing: 16) {
                    Image(systemName: "wifi.exclamationmark").font(.largeTitle).foregroundStyle(.secondary)
                    Text(opened.node.name).font(.headline)
                    Text(failure).multilineTextAlignment(.center).foregroundStyle(.secondary)
                    HStack {
                        Button("Volver") { dismiss() }.buttonStyle(.bordered)
                        Button("Reintentar") { page.retry() }.buttonStyle(.borderedProminent)
                    }
                }
                .padding(32)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .background(Color(uiColor: .systemBackground).ignoresSafeArea())
            }
        }
        .toolbar(.hidden, for: .navigationBar)
        .navigationBarBackButtonHidden()
    }
}

/// Owns the web view of a node: pinned TLS, links that stay inside and web dialogs.
@MainActor
final class NodePage: NSObject, ObservableObject, WKNavigationDelegate, WKUIDelegate {
    let opened: OpenedNode
    let webView: WKWebView
    @Published var failure: String?
    @Published var background: UIColor = .systemBackground
    weak var presenter: UIViewController?
    private var observation: NSKeyValueObservation?

    init(opened: OpenedNode) {
        self.opened = opened
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .default()
        configuration.allowsInlineMediaPlayback = true
        configuration.mediaTypesRequiringUserActionForPlayback = []
        webView = WKWebView(frame: .zero, configuration: configuration)
        super.init()
        webView.navigationDelegate = self
        webView.uiDelegate = self
        webView.allowsBackForwardNavigationGestures = true
        webView.scrollView.contentInsetAdjustmentBehavior = .never
        webView.isOpaque = false
        webView.backgroundColor = .systemBackground
        if #available(iOS 16.4, *) { webView.isInspectable = true }
        observation = webView.observe(\.underPageBackgroundColor, options: [.new]) { [weak self] web, _ in
            let color = web.underPageBackgroundColor ?? .systemBackground
            Task { @MainActor in self?.background = color }
        }
        webView.load(URLRequest(url: opened.url))
    }

    func retry() {
        failure = nil
        if webView.url == nil { webView.load(URLRequest(url: opened.url)) } else { webView.reload() }
    }

    // MARK: WKNavigationDelegate

    /// A node's self-signed certificate is accepted only if its key is the one
    /// pinned when the phone paired with it.
    func webView(_ webView: WKWebView, didReceive challenge: URLAuthenticationChallenge,
                 completionHandler: @escaping @MainActor (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        let space = challenge.protectionSpace
        guard space.authenticationMethod == NSURLAuthenticationMethodServerTrust, let trust = space.serverTrust,
              let want = Pin.pinned(host: space.host, port: space.port) else {
            completionHandler(.performDefaultHandling, nil)
            return
        }
        if Pin.of(trust) == want {
            completionHandler(.useCredential, URLCredential(trust: trust))
        } else {
            completionHandler(.cancelAuthenticationChallenge, nil)
        }
    }

    /// Links inside an open node stay in the app; the rest open in Safari.
    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction,
                 decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = navigationAction.request.url else {
            decisionHandler(.allow)
            return
        }
        let scheme = url.scheme?.lowercased() ?? ""
        let embedded = navigationAction.targetFrame != nil && navigationAction.targetFrame?.isMainFrame == false
        if Pin.pinned(url) != nil || ["about", "blob", "data"].contains(scheme) || embedded {
            decisionHandler(.allow)
            return
        }
        UIApplication.shared.open(url)
        decisionHandler(.cancel)
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        failure = nil
        background = webView.underPageBackgroundColor ?? .systemBackground
    }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        let code = (error as NSError).code
        if code == NSURLErrorCancelled || code == 102 { return } // 102: frame load interrupted by a policy change
        failure = error.localizedDescription
    }

    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        webView.reload()
    }

    // MARK: WKUIDelegate

    /// Microphone and camera only for the page of a paired node, and only with its pinned key.
    func webView(_ webView: WKWebView, requestMediaCapturePermissionFor origin: WKSecurityOrigin,
                 initiatedByFrame frame: WKFrameInfo, type: WKMediaCaptureType,
                 decisionHandler: @escaping (WKPermissionDecision) -> Void) {
        let port = origin.port == 0 ? 443 : origin.port
        let want = origin.protocol == "https" ? Pin.pinned(host: origin.host, port: port) : nil
        let trusted = want != nil && webView.url?.host == origin.host && Pin.of(webView.serverTrust) == want
        decisionHandler(trusted ? .grant : .deny)
    }

    /// Pages that open a new window: the node stays here, the rest goes to Safari.
    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration,
                 for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        if let url = navigationAction.request.url {
            if Pin.pinned(url) != nil { webView.load(navigationAction.request) } else { UIApplication.shared.open(url) }
        }
        return nil
    }

    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String,
                 initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        let alert = UIAlertController(title: nil, message: message, preferredStyle: .alert)
        alert.addAction(UIAlertAction(title: "Aceptar", style: .default) { _ in completionHandler() })
        if !present(alert) { completionHandler() }
    }

    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String,
                 initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        let alert = UIAlertController(title: nil, message: message, preferredStyle: .alert)
        alert.addAction(UIAlertAction(title: "Cancelar", style: .cancel) { _ in completionHandler(false) })
        alert.addAction(UIAlertAction(title: "Aceptar", style: .default) { _ in completionHandler(true) })
        if !present(alert) { completionHandler(false) }
    }

    func webView(_ webView: WKWebView, runJavaScriptTextInputPanelWithPrompt prompt: String, defaultText: String?,
                 initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (String?) -> Void) {
        let alert = UIAlertController(title: nil, message: prompt, preferredStyle: .alert)
        alert.addTextField { $0.text = defaultText }
        alert.addAction(UIAlertAction(title: "Cancelar", style: .cancel) { _ in completionHandler(nil) })
        alert.addAction(UIAlertAction(title: "Aceptar", style: .default) { [weak alert] _ in
            completionHandler(alert?.textFields?.first?.text ?? "")
        })
        if !present(alert) { completionHandler(nil) }
    }

    private func present(_ alert: UIAlertController) -> Bool {
        guard let presenter, presenter.view.window != nil, presenter.presentedViewController == nil else { return false }
        presenter.present(alert, animated: true)
        return true
    }
}

/// Hosts the web view between the safe areas, above the keyboard, and lets the
/// edge swipe go back in the node's history or, on its first page, to the list.
struct NodeContainer: UIViewControllerRepresentable {
    let page: NodePage

    func makeUIViewController(context: Context) -> NodeViewController {
        NodeViewController(page: page)
    }

    func updateUIViewController(_ controller: NodeViewController, context: Context) {}
}

final class NodeViewController: UIViewController, UIGestureRecognizerDelegate {
    private let page: NodePage
    private weak var popGesture: UIGestureRecognizer?
    private weak var popDelegate: UIGestureRecognizerDelegate?

    init(page: NodePage) {
        self.page = page
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) { fatalError("init(coder:) is not used") }

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .clear
        let web = page.webView
        web.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(web)
        NSLayoutConstraint.activate([
            web.topAnchor.constraint(equalTo: view.safeAreaLayoutGuide.topAnchor),
            web.leadingAnchor.constraint(equalTo: view.safeAreaLayoutGuide.leadingAnchor),
            web.trailingAnchor.constraint(equalTo: view.safeAreaLayoutGuide.trailingAnchor),
            web.bottomAnchor.constraint(equalTo: view.keyboardLayoutGuide.topAnchor),
        ])
        page.presenter = self
    }

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        guard let gesture = navigationController?.interactivePopGestureRecognizer else { return }
        if gesture.delegate !== self {
            popDelegate = gesture.delegate
            gesture.delegate = self
        }
        popGesture = gesture
        gesture.isEnabled = true
    }

    override func viewWillDisappear(_ animated: Bool) {
        super.viewWillDisappear(animated)
        if let gesture = popGesture, gesture.delegate === self { gesture.delegate = popDelegate }
    }

    func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
        !page.webView.canGoBack
    }
}
