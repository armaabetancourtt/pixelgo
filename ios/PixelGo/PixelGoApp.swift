import SwiftUI
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
            await reload()
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

    private func authenticate(
        operation: () async throws -> Void
    ) async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            try await operation()
            isAuthenticated = true
            await reload()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
