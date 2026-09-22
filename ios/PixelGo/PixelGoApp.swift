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
                    await model.reload()
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
    @Published var isLoading = false
    @Published var errorMessage: String?

    private let api = APIClient(baseURL: URL(string: "http://localhost:8080")!)

    func reload() async {
        isLoading = true
        defer { isLoading = false }
        do {
            async let loadedDevices = api.listDevices()
            async let loadedTransfers = api.listTransfers()
            devices = try await loadedDevices
            transfers = try await loadedTransfers
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
