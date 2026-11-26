import AVFoundation
import Speech
import SwiftUI

/// Dictates an idea with Apple's speech recognizer and saves it in the capture
/// node. A pause in speech does not send it; leaving keeps the draft.
@MainActor
final class IdeaCapture: ObservableObject {
    enum Phase: Equatable { case starting, listening, paused, failed, saving, saveFailed, saved }

    @Published private(set) var phase: Phase = .starting
    @Published private(set) var status = "Activando micrófono…"
    @Published private(set) var committed = ""
    @Published private(set) var partial = ""
    @Published private(set) var level: CGFloat = 0
    @Published private(set) var destination = ""

    var text: String { Self.join(committed, partial) }
    var canSend: Bool {
        !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && phase != .saving && phase != .saved
    }

    private let engine = AVAudioEngine()
    private var request: SFSpeechAudioBufferRecognitionRequest?
    private var task: SFSpeechRecognitionTask?
    private var generation = 0
    private var silentRestarts = 0
    private var ideaId = UUID().uuidString
    private var retryNode: String?
    private var started = false

    private static let draftKey = "capture.draft"
    private static let draftLifetime: TimeInterval = 12 * 60 * 60

    func start() async {
        guard !started else { return }
        started = true
        restoreDraft()
        do {
            destination = try AppSettings.captureDestination(retry: retryNode).name
        } catch {
            phase = .failed
            status = error.localizedDescription
            return
        }
        await resume()
    }

    func resume() async {
        phase = .starting
        status = "Activando micrófono…"
        guard await authorize() else {
            phase = .failed
            status = "Permite el micrófono y el reconocimiento de voz en los ajustes de iOS para dictar."
            return
        }
        silentRestarts = 0
        listen()
    }

    func pause() {
        guard phase == .listening || phase == .starting else { return }
        stopAudio(release: true)
        committed = text
        partial = ""
        saveDraft()
        phase = .paused
        status = "En pausa"
    }

    func discard() {
        stopAudio(release: true)
        clearDraft()
    }

    /// Sends the idea; the same id makes a retry idempotent.
    func send() async -> Bool {
        let idea = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !idea.isEmpty, phase != .saving, phase != .saved else { return false }
        stopAudio(release: true)
        committed = idea
        partial = ""
        phase = .saving
        status = "Guardando…"
        do {
            let node = try AppSettings.captureDestination(retry: retryNode)
            retryNode = node.id
            saveDraft()
            try await NodeClient.sendIdea(node, id: ideaId, text: idea)
            clearDraft()
            phase = .saved
            status = "Idea guardada en \(node.name)"
            return true
        } catch {
            saveDraft()
            phase = .saveFailed
            status = error.localizedDescription
            return false
        }
    }

    private func authorize() async -> Bool {
        let speech: SFSpeechRecognizerAuthorizationStatus = await withCheckedContinuation { continuation in
            SFSpeechRecognizer.requestAuthorization { continuation.resume(returning: $0) }
        }
        guard speech == .authorized else { return false }
        return await AVAudioApplication.requestRecordPermission()
    }

    private func listen() {
        stopAudio(release: false)
        guard let recognizer = SFSpeechRecognizer() ?? SFSpeechRecognizer(locale: Locale(identifier: "es-ES")),
              recognizer.isAvailable else {
            phase = .failed
            status = "No hay un reconocedor de voz disponible."
            return
        }
        let request = SFSpeechAudioBufferRecognitionRequest()
        request.shouldReportPartialResults = true
        request.addsPunctuation = true
        if AppSettings.offlineSpeech && recognizer.supportsOnDeviceRecognition {
            request.requiresOnDeviceRecognition = true
        }
        do {
            let session = AVAudioSession.sharedInstance()
            try session.setCategory(.record, mode: .measurement, options: [.duckOthers])
            try session.setActive(true, options: .notifyOthersOnDeactivation)
            let input = engine.inputNode
            let format = input.outputFormat(forBus: 0)
            guard format.sampleRate > 0, format.channelCount > 0 else { throw DoryError("Sin micrófono") }
            input.installTap(onBus: 0, bufferSize: 1024, format: format, block: Self.tap(request, owner: self))
            engine.prepare()
            try engine.start()
        } catch {
            stopAudio(release: true)
            phase = .failed
            status = "No se pudo activar el micrófono."
            return
        }
        generation += 1
        self.request = request
        task = recognizer.recognitionTask(with: request, resultHandler: Self.handler(owner: self, generation: generation))
        phase = .listening
        status = "Escuchando…"
    }

    private func recognized(_ current: Int, text heard: String?, final: Bool, failed: Bool) {
        guard current == generation, phase == .listening else { return }
        if let heard, !heard.isEmpty {
            // Some versions of iOS start the transcript over after a pause: keep what came before.
            if heard.count < partial.count / 2 {
                committed = Self.join(committed, partial)
            }
            partial = heard
            silentRestarts = 0
        }
        guard final || failed else { return }
        let hadSpeech = !partial.isEmpty
        committed = Self.join(committed, partial)
        partial = ""
        saveDraft()
        // Silence or the length limit of a request ends it: keep listening, but
        // pause after a while of silence so the microphone does not stay open.
        if !hadSpeech { silentRestarts += 1 }
        if silentRestarts >= 3 {
            stopAudio(release: true)
            phase = .paused
            status = "En pausa: toca el micrófono para seguir"
            return
        }
        listen()
    }

    private func stopAudio(release: Bool) {
        generation += 1
        task?.cancel()
        task = nil
        request?.endAudio()
        request = nil
        if engine.isRunning { engine.stop() }
        engine.inputNode.removeTap(onBus: 0)
        if release { try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation) }
        level = 0
    }

    private func saveDraft() {
        let draft: [String: Any] = ["id": ideaId, "text": text, "node": retryNode ?? "", "at": Date().timeIntervalSince1970]
        UserDefaults.standard.set(draft, forKey: Self.draftKey)
    }

    private func restoreDraft() {
        guard let draft = UserDefaults.standard.dictionary(forKey: Self.draftKey),
              let at = draft["at"] as? Double, Date().timeIntervalSince1970 - at < Self.draftLifetime,
              let id = draft["id"] as? String, let text = draft["text"] as? String, !text.isEmpty else {
            clearDraft()
            return
        }
        ideaId = id
        committed = text
        if let node = draft["node"] as? String, !node.isEmpty { retryNode = node }
    }

    private func clearDraft() {
        UserDefaults.standard.removeObject(forKey: Self.draftKey)
    }

    // The audio and recognition callbacks run off the main thread.

    nonisolated private static func tap(_ request: SFSpeechAudioBufferRecognitionRequest,
                                        owner: IdeaCapture) -> AVAudioNodeTapBlock {
        { [weak owner] buffer, _ in
            request.append(buffer)
            let value = loudness(buffer)
            Task { @MainActor in owner?.level = value }
        }
    }

    nonisolated private static func handler(owner: IdeaCapture, generation: Int) -> @Sendable (SFSpeechRecognitionResult?, Error?) -> Void {
        { [weak owner] result, error in
            let heard = result?.bestTranscription.formattedString
            let final = result?.isFinal ?? false
            let failed = error != nil
            Task { @MainActor in owner?.recognized(generation, text: heard, final: final, failed: failed) }
        }
    }

    nonisolated private static func loudness(_ buffer: AVAudioPCMBuffer) -> CGFloat {
        guard let samples = buffer.floatChannelData?[0], buffer.frameLength > 0 else { return 0 }
        let count = Int(buffer.frameLength)
        var sum: Float = 0
        for i in 0..<count { sum += samples[i] * samples[i] }
        let decibels = 20 * log10(max(sqrt(sum / Float(count)), 0.000_001))
        return CGFloat(max(0, min(1, (decibels + 50) / 40)))
    }

    nonisolated static func join(_ first: String, _ second: String) -> String {
        let a = first.trimmingCharacters(in: .whitespaces)
        let b = second.trimmingCharacters(in: .whitespaces)
        if a.isEmpty { return b }
        if b.isEmpty { return a }
        return "\(a) \(b)"
    }
}

struct CaptureView: View {
    @StateObject private var capture = IdeaCapture()
    @Environment(\.dismiss) private var dismiss
    @Environment(\.scenePhase) private var scenePhase

    private static let ink = Color(red: 0x0b / 255, green: 0x12 / 255, blue: 0x20 / 255)
    private static let accent = Color(red: 0x93 / 255, green: 0xc5 / 255, blue: 0xfd / 255)
    private static let muted = Color(red: 0xaa / 255, green: 0xb7 / 255, blue: 0xc8 / 255)
    private static let good = Color(red: 0x4a / 255, green: 0xde / 255, blue: 0x80 / 255)
    private static let bad = Color(red: 0xf8 / 255, green: 0x71 / 255, blue: 0x71 / 255)

    var body: some View {
        ZStack {
            Self.ink.ignoresSafeArea()
            VStack(spacing: 20) {
                Text(capture.destination.isEmpty ? "Nueva idea" : "Idea para \(capture.destination)")
                    .font(.subheadline)
                    .foregroundStyle(Self.muted)
                    .frame(maxWidth: .infinity, alignment: .leading)
                ScrollViewReader { proxy in
                    ScrollView {
                        Text(capture.text.isEmpty ? "Habla para dictar tu idea…" : capture.text)
                            .font(.title2)
                            .foregroundStyle(capture.text.isEmpty ? Self.muted : .white)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .textSelection(.enabled)
                            .id("end")
                    }
                    .onChange(of: capture.text) { _, _ in proxy.scrollTo("end", anchor: .bottom) }
                }
                Text(capture.status)
                    .font(.callout)
                    .foregroundStyle(statusColor)
                    .multilineTextAlignment(.center)
                HStack(spacing: 36) {
                    roundButton("xmark", label: "Descartar", size: 56, filled: false) {
                        capture.discard()
                        dismiss()
                    }
                    .disabled(capture.phase == .saving)
                    ZStack {
                        Circle()
                            .stroke(Self.accent.opacity(0.35), lineWidth: 6)
                            .frame(width: 92, height: 92)
                            .scaleEffect(1 + capture.level * 0.35)
                            .animation(.easeOut(duration: 0.15), value: capture.level)
                        roundButton(primaryIcon, label: primaryLabel, size: 80, filled: true) { primary() }
                            .disabled(capture.phase == .saving || capture.phase == .saved || capture.phase == .starting)
                    }
                    roundButton("paperplane.fill", label: "Enviar", size: 56, filled: false) { send() }
                        .disabled(!capture.canSend)
                }
                .padding(.bottom, 12)
            }
            .padding(24)
        }
        .preferredColorScheme(.dark)
        .task { await capture.start() }
        .onChange(of: scenePhase) { _, phase in
            if phase != .active { capture.pause() }
        }
        .onDisappear { capture.pause() }
    }

    private var statusColor: Color {
        switch capture.phase {
        case .failed, .saveFailed: return Self.bad
        case .saved: return Self.good
        default: return Self.muted
        }
    }

    private var primaryIcon: String {
        switch capture.phase {
        case .listening: return "pause.fill"
        case .saveFailed: return "arrow.clockwise"
        default: return "mic.fill"
        }
    }

    private var primaryLabel: String {
        switch capture.phase {
        case .listening: return "Pausar"
        case .saveFailed: return "Reintentar"
        default: return "Continuar"
        }
    }

    private func primary() {
        switch capture.phase {
        case .listening: capture.pause()
        case .saveFailed: send()
        default: Task { await capture.resume() }
        }
    }

    private func send() {
        Task {
            if await capture.send() {
                try? await Task.sleep(nanoseconds: 900_000_000)
                dismiss()
            }
        }
    }

    private func roundButton(_ icon: String, label: String, size: CGFloat, filled: Bool,
                             action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: icon)
                .font(.system(size: size * 0.36, weight: .semibold))
                .frame(width: size, height: size)
                .foregroundStyle(filled ? Self.ink : .white)
                .background(Circle().fill(filled ? Self.accent : Color.white.opacity(0.12)))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(label)
    }
}
