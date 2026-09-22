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
    @Published var onlineDeviceIDs: Set<String> = []
    @Published var isLoading = false
    @Published var errorMessage: String?

    private let api = APIClient(baseURL: URL(string: "http://localhost:8080")!)

    func reload() async {
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
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
