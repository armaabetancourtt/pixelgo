import Foundation
import SwiftUI
import UIKit
import UserNotifications

@main
struct PixelGoApp: App {
    @UIApplicationDelegateAdaptor(PushAppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel()

    init() {
        TransferBackgroundCoordinator.shared.register()
    }

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(model)
                .task {
                    await model.bootstrap()
                    await requestNotifications()
                }
        }
    }

    private func requestNotifications() async {
        let granted = (try? await UNUserNotificationCenter.current()
            .requestAuthorization(options: [.alert, .badge, .sound])) ?? false

        if granted {
            await MainActor.run {
                UIApplication.shared.registerForRemoteNotifications()
            }
        }
    }
}

@MainActor
final class AppModel: ObservableObject {
    @Published var devices: [PixelDevice] = []
    @Published var transfers: [Transfer] = []
    @Published var onlineDeviceIDs: Set<String> = []
    @Published var receivedItems: [ReceivedTextItem] = []
    @Published var localDeviceID: String?
    @Published var isLoading = false
    @Published var isAuthenticated = false
    @Published var didBootstrap = false
    @Published var errorMessage: String?

    private let api: APIClient
    private var realtimeTask: Task<Void, Never>?
    private var pushTokenTask: Task<Void, Never>?

    init() {
        let sessionStore = SessionStore()
        self.api = APIClient(
            baseURL: URL(string: "http://localhost:8080")!,
            sessionStore: sessionStore
        )

        self.pushTokenTask = Task { [weak self] in
            let stream = await PushTokenBroker.shared.stream()
            for await token in stream {
                guard !Task.isCancelled else { break }
                await self?.syncPushToken(token)
            }
        }
    }

    func bootstrap() async {
        isAuthenticated = await api.hasStoredSession()
        didBootstrap = true
        if isAuthenticated {
            await prepareAuthenticatedSession()
        }
    }

    func login(email: String, password: String) async {
        await authenticate {
            try await api.login(email: email, password: password)
        }
    }

    func register(email: String, password: String) async {
        await authenticate {
            try await api.register(email: email, password: password)
        }
    }

    func signOut() async {
        realtimeTask?.cancel()
        realtimeTask = nil
        await api.signOut()
        devices = []
        transfers = []
        onlineDeviceIDs = []
        receivedItems = []
        localDeviceID = nil
        errorMessage = nil
        isAuthenticated = false
    }

    func reload() async {
        guard isAuthenticated else { return }

        isLoading = true
        defer { isLoading = false }
        do {
            async let devicesTask = api.listDevices()
            async let transfersTask = api.listTransfers()

            let loadedDevices = try await devicesTask
            devices = loadedDevices
            transfers = try await transfersTask

            var onlineIDs = Set<String>()
            for device in loadedDevices {
                if let presence = try? await api.devicePresence(device.id), presence.online {
                    onlineIDs.insert(device.id)
                }
            }
            onlineDeviceIDs = onlineIDs
            errorMessage = nil
        } catch APIClient.APIError.refreshFailed {
            await signOut()
            errorMessage = "Your session expired. Sign in again."
        } catch APIClient.APIError.noSession {
            await signOut()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func sendText(
        _ text: String,
        to destinationDeviceID: String
    ) async -> Bool {
        guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return false
        }

        return await performSend { sourceDeviceID in
            try await api.sendText(
                text,
                sourceDeviceID: sourceDeviceID,
                destinationDeviceID: destinationDeviceID
            )
        }
    }

    func sendPayload(
        _ data: Data,
        kind: Transfer.Kind,
        displayName: String,
        contentType: String,
        to destinationDeviceID: String
    ) async -> Bool {
        guard !data.isEmpty else { return false }

        return await performSend { sourceDeviceID in
            try await api.sendPayload(
                data,
                kind: kind,
                displayName: displayName,
                contentType: contentType,
                sourceDeviceID: sourceDeviceID,
                destinationDeviceID: destinationDeviceID
            )
        }
    }

    private func performSend(
        operation: (String) async throws -> Transfer
    ) async -> Bool {
        guard let sourceDeviceID = localDeviceID else {
            return false
        }

        isLoading = true
        defer { isLoading = false }

        do {
            _ = try await operation(sourceDeviceID)
            await reload()
            return true
        } catch APIClient.APIError.refreshFailed {
            await signOut()
            errorMessage = "Your session expired. Sign in again."
            return false
        } catch {
            errorMessage = error.localizedDescription
            return false
        }
    }

    func downloadPayload(for transfer: Transfer) async -> Data? {
        isLoading = true
        defer { isLoading = false }

        do {
            return try await api.downloadPayload(for: transfer)
        } catch APIClient.APIError.refreshFailed {
            await signOut()
            errorMessage = "Your session expired. Sign in again."
            return nil
        } catch {
            errorMessage = error.localizedDescription
            return nil
        }
    }

    func completeIncomingTransfer(_ transferID: String) async {
        do {
            _ = try await api.completeTransfer(transferID)
            await reload()
        } catch APIClient.APIError.refreshFailed {
            await signOut()
            errorMessage = "Your session expired. Sign in again."
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func receivePendingItems() async {
        guard let localDeviceID else { return }

        do {
            let incoming = try await api.receiveReadyTextItems(
                destinationDeviceID: localDeviceID
            )
            guard !incoming.isEmpty else { return }

            var knownIDs = Set(receivedItems.map(\.id))
            for item in incoming where !knownIDs.contains(item.id) {
                receivedItems.insert(item, at: 0)
                knownIDs.insert(item.id)
            }
        } catch APIClient.APIError.refreshFailed {
            await signOut()
            errorMessage = "Your session expired. Sign in again."
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func prepareAuthenticatedSession() async {
        do {
            let device = try await api.ensureCurrentDevice(
                name: UIDevice.current.name,
                platform: "ios"
            )
            localDeviceID = device.id

            if let token = await PushTokenBroker.shared.current() {
                await syncPushToken(token)
            }

            // Durable state is the recovery path if realtime was missed while
            // the app was suspended or offline.
            await receivePendingItems()

            startRealtime(deviceID: device.id)
            await reload()
        } catch APIClient.APIError.refreshFailed {
            await signOut()
            errorMessage = "Your session expired. Sign in again."
        } catch APIClient.APIError.noSession {
            await signOut()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func syncPushToken(_ token: String) async {
        guard
            isAuthenticated,
            let deviceID = localDeviceID,
            !token.isEmpty
        else {
            return
        }

        do {
            _ = try await api.updatePushToken(token, deviceID: deviceID)
        } catch APIClient.APIError.refreshFailed {
            await signOut()
            errorMessage = "Your session expired. Sign in again."
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func startRealtime(deviceID: String) {
        realtimeTask?.cancel()

        realtimeTask = Task { [weak self] in
            guard let self else { return }

            while !Task.isCancelled && self.isAuthenticated {
                do {
                    try await self.api.openEvents(deviceID: deviceID)

                    while !Task.isCancelled {
                        let event = try await self.api.nextEvent()
                        if event.type == "transfer.ready" {
                            await self.receivePendingItems()
                        }
                        await self.reload()
                    }
                } catch {
                    if Task.isCancelled { break }

                    // Force an authenticated request before reconnecting. If
                    // the access token expired, APIClient rotates refresh once
                    // and the next WebSocket handshake uses the new token.
                    _ = try? await self.api.listDevices()
                    try? await Task.sleep(nanoseconds: 1_000_000_000)
                }
            }
        }
    }

    private func authenticate(
        operation: () async throws -> Void
    ) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            try await operation()
            isAuthenticated = true
            await prepareAuthenticatedSession()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
