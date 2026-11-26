import BackgroundTasks
import UIKit
import UserNotifications

/// Native notifications for the notices of the mesh. Without a paid Apple
/// account there is no push and iOS does not keep a connection open in the
/// background, so the app checks the notices of a paired node when iOS grants
/// it background time, every 15 minutes at best.
enum Notices {
    static let taskIdentifier = "net.dorlab.dory.notices"
    private static let seenKey = "notices.seen"
    /// Notices older than this are not shown: iOS may wait hours between checks.
    private static let freshness: TimeInterval = 6 * 60 * 60

    static func register() {
        BGTaskScheduler.shared.register(forTaskWithIdentifier: taskIdentifier, using: nil) { task in
            guard let task = task as? BGAppRefreshTask else {
                task.setTaskCompleted(success: false)
                return
            }
            schedule()
            let work = Task {
                await checkAll()
                task.setTaskCompleted(success: true)
            }
            task.expirationHandler = { work.cancel() }
        }
    }

    static func schedule() {
        guard AppSettings.noticesEnabled, !NodeStore.load().isEmpty else {
            BGTaskScheduler.shared.cancel(taskRequestWithIdentifier: taskIdentifier)
            return
        }
        let request = BGAppRefreshTaskRequest(identifier: taskIdentifier)
        request.earliestBeginDate = Date(timeIntervalSinceNow: 15 * 60)
        try? BGTaskScheduler.shared.submit(request)
    }

    static func requestPermission() {
        guard AppSettings.noticesEnabled else { return }
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { _, _ in }
    }

    /// Tries the paired nodes in turn; notices replicate, so any one is enough.
    static func checkAll() async {
        guard AppSettings.noticesEnabled else { return }
        for node in NodeStore.load() where !node.pin.isEmpty && !node.token.isEmpty {
            if Task.isCancelled { return }
            do {
                try await check(node)
                return
            } catch {
                // Unreachable node: try the next one.
            }
        }
    }

    /// Shows the fresh unread notices not shown yet, following the mode of each kind.
    static func check(_ node: Node) async throws {
        guard let notices = try await NodeClient.json(node, "/api/notices") as? [[String: Any]] else { return }
        let defaults = UserDefaults.standard
        let firstCheck = defaults.object(forKey: seenKey) == nil
        var seen = defaults.stringArray(forKey: seenKey) ?? []
        let known = Set(seen)
        let now = Date()
        let fresh = notices.filter { notice in
            guard let id = notice["id"] as? String, !known.contains(id) else { return false }
            if let read = notice["read_at"], !(read is NSNull) { return false }
            guard let created = Dates.parse(notice["created_at"] as? String) else { return false }
            return now.timeIntervalSince(created) < freshness
        }
        if fresh.isEmpty { return }
        var modes: [String: String] = [:]
        for setting in try await NodeClient.json(node, "/api/notice-settings") as? [[String: Any]] ?? [] {
            if let kind = setting["kind"] as? String, let mode = setting["mode"] as? String { modes[kind] = mode }
        }
        // An interface in focus anywhere in the mesh (the desktop, say) means the
        // user is at Dory already: the notice shows there, not on the phone.
        var focus = await MainActor.run { UIApplication.shared.applicationState == .active }
        if !focus, let presence = try? await NodeClient.json(node, "/api/presence") as? [String: Any] {
            focus = presence["focus"] as? Bool ?? false
        }
        for notice in fresh {
            guard let id = notice["id"] as? String else { continue }
            seen.append(id)
            // The first check only learns what is there, so enabling notices does not flood the phone.
            if firstCheck { continue }
            let mode = modes[notice["kind"] as? String ?? ""] ?? "auto"
            if mode == "off" || mode == "app" || (mode == "auto" && focus) { continue }
            await show(id: id, notice: notice, node: node)
        }
        if seen.count > 300 { seen.removeFirst(seen.count - 300) }
        defaults.set(seen, forKey: seenKey)
    }

    private static func show(id: String, notice: [String: Any], node: Node) async {
        let content = UNMutableNotificationContent()
        content.title = notice["title"] as? String ?? "Dory"
        content.body = notice["body"] as? String ?? ""
        content.sound = .default
        content.threadIdentifier = node.id
        content.userInfo = ["node": node.id, "url": notice["url"] as? String ?? "/"]
        // Same id as the notice, so a later check never shows it twice.
        try? await UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: id, content: content, trigger: nil))
    }
}
