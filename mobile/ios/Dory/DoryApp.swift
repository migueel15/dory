import SwiftUI
import UserNotifications

@main
struct DoryApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @StateObject private var model = AppModel.shared

    var body: some Scene {
        WindowGroup {
            RootView().environmentObject(model)
        }
    }
}

final class AppDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        // Background tasks must be registered before launch ends.
        Notices.register()
        return true
    }

    /// A notice that arrives while the app is open still shows, like on Android with "always push".
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification,
                                withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .list, .sound])
    }

    /// A tapped notice opens its node on the page it points to.
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse,
                                withCompletionHandler completionHandler: @escaping () -> Void) {
        let info = response.notification.request.content.userInfo
        if let node = info["node"] as? String {
            let url = info["url"] as? String ?? "/"
            Task { @MainActor in await AppModel.shared.openNotice(node: node, url: url) }
        }
        completionHandler()
    }
}
