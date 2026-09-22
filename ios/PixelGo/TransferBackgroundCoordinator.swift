import BackgroundTasks

@MainActor
final class TransferBackgroundCoordinator {
    static let shared = TransferBackgroundCoordinator()
    static let refreshIdentifier = "com.armaabetancourtt.pixelgo.transfer.refresh"

    private init() {}

    func register() {
        BGTaskScheduler.shared.register(
            forTaskWithIdentifier: Self.refreshIdentifier,
            using: nil
        ) { [weak self] task in
            Task { @MainActor in
                guard let self else {
                    task.setTaskCompleted(success: false)
                    return
                }
                guard let refreshTask = task as? BGAppRefreshTask else {
                    task.setTaskCompleted(success: false)
                    return
                }
                self.handle(refreshTask)
            }
        }
    }

    func schedule() {
        let request = BGAppRefreshTaskRequest(identifier: Self.refreshIdentifier)
        try? BGTaskScheduler.shared.submit(request)
    }

    private func handle(_ task: BGAppRefreshTask) {
        schedule()
        task.expirationHandler = {}
        task.setTaskCompleted(success: true)
    }
}
