import SwiftUI
import UIKit
import UserNotifications

@main
struct PixelGoApp: App {
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
        _ = try? await UNUserNotificationCenter.current()
            .requestAuthorization(options: [.alert, .badge, .sound])
    }
}

@MainActor
final class AppModel: ObservableObject {
    @Published var devices: [PixelDevice] = []
    @Published var transfers: [Transfer] = []
    @Published var onlineDeviceIDs: Set<String> = []
    @Published var isLoading = false
    @Published var isAuthenticated = false
    @Published var didBootstrap = false
    @Published var errorMessage: String?

    private let api: APIClient
    private var realtimeTask: Task<Void, Never>?

    init() {
        let sessionStore = SessionStore()
        self.api = APIClient(
            baseURL: URL(string: "http://localhost:8080")!,
            sessionStore: sessionStore
        )
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

    private func prepareAuthenticatedSession() async {
        do {
            let device = try await api.ensureCurrentDevice(
                name: UIDevice.current.name,
                platform: "ios"
            )
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

    private func startRealtime(deviceID: String) {
        realtimeTask?.cancel()

        realtimeTask = Task { [weak self] in
            guard let self else { return }

            while !Task.isCancelled && self.isAuthenticated {
                do {
                    try await self.api.openEvents(deviceID: deviceID)

                    while !Task.isCancelled {
                        _ = try await self.api.nextEvent()
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
