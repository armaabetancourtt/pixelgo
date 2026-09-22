import CryptoKit
import Foundation

actor APIClient {
    enum APIError: LocalizedError {
        case invalidResponse
        case server(status: Int)
        case noSession
        case refreshFailed
        case realtimeNotConnected
        case checksumMismatch

        var errorDescription: String? {
            switch self {
            case .invalidResponse:
                return "The server returned an invalid response."
            case .server(let status):
                return "PIXEL GO API returned HTTP \(status)."
            case .noSession:
                return "Sign in to continue."
            case .refreshFailed:
                return "Your session expired. Sign in again."
            case .realtimeNotConnected:
                return "Realtime connection is not active."
            case .checksumMismatch:
                return "The received payload failed integrity verification."
            }
        }
    }

    private let baseURL: URL
    private let session: URLSession
    private let sessionStore: SessionStore
    private let decoder: JSONDecoder
    private let encoder = JSONEncoder()

    private var refreshTask: Task<TokenPair, Error>?
    private var webSocketTask: URLSessionWebSocketTask?

    init(
        baseURL: URL,
        session: URLSession = .shared,
        sessionStore: SessionStore
    ) {
        self.baseURL = baseURL
        self.session = session
        self.sessionStore = sessionStore
        self.decoder = JSONDecoder()
        self.decoder.dateDecodingStrategy = .iso8601
    }

    func hasStoredSession() async -> Bool {
        (try? await sessionStore.load()) != nil
    }

    func register(email: String, password: String) async throws {
        let pair: TokenPair = try await publicPost(
            "/v1/auth/register",
            body: AuthCredentials(email: email, password: password)
        )
        try await sessionStore.save(pair)
    }

    func login(email: String, password: String) async throws {
        let pair: TokenPair = try await publicPost(
            "/v1/auth/login",
            body: AuthCredentials(email: email, password: password)
        )
        try await sessionStore.save(pair)
    }

    func signOut() async {
        closeEvents()
        refreshTask?.cancel()
        refreshTask = nil
        try? await sessionStore.clear()
    }

    func ensureCurrentDevice(
        name: String,
        platform: String
    ) async throws -> PixelDevice {
        guard let pair = try await sessionStore.load() else {
            throw APIError.noSession
        }

        if let storedID = try await sessionStore.deviceID(for: pair.userId) {
            let devices: [PixelDevice] = try await listDevices()
            if let existing = devices.first(where: { $0.id == storedID }) {
                return existing
            }
        }

        let idempotencyKey = try await sessionStore.registrationKey(
            for: pair.userId
        )
        let body = RegisterDeviceRequest(
            name: String(name.prefix(120)),
            platform: platform,
            pushToken: nil
        )

        let device: PixelDevice = try await authenticatedPost(
            "/v1/devices",
            body: body,
            idempotencyKey: idempotencyKey
        )
        try await sessionStore.saveDeviceID(device.id, for: pair.userId)
        return device
    }

    func updatePushToken(
        _ token: String,
        deviceID: String
    ) async throws -> PixelDevice {
        var request = URLRequest(
            url: baseURL.appending(path: "/v1/devices/\(deviceID)/push-token")
        )
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try encoder.encode(PushTokenRequest(pushToken: token))
        return try await performAuthenticated(request, retryAfterRefresh: true)
    }

    func sendText(
        _ text: String,
        sourceDeviceID: String,
        destinationDeviceID: String
    ) async throws -> Transfer {
        let kind = inferredTextKind(text)
        return try await sendPayload(
            Data(text.utf8),
            kind: kind,
            displayName: kind == .link ? "Link" : "Text",
            contentType: "text/plain; charset=utf-8",
            sourceDeviceID: sourceDeviceID,
            destinationDeviceID: destinationDeviceID
        )
    }

    func sendPayload(
        _ payload: Data,
        kind: Transfer.Kind,
        displayName: String,
        contentType: String,
        sourceDeviceID: String,
        destinationDeviceID: String
    ) async throws -> Transfer {
        let checksum = sha256Hex(payload)

        let created: Transfer = try await authenticatedPost(
            "/v1/transfers",
            body: CreateTransferRequest(
                sourceDeviceId: sourceDeviceID,
                destinationDeviceId: destinationDeviceID,
                kind: kind.rawValue,
                displayName: String(displayName.prefix(255)),
                contentType: String(contentType.prefix(255)),
                sizeBytes: Int64(payload.count),
                sha256: checksum
            ),
            idempotencyKey: "transfer-create-\(UUID().uuidString.lowercased())"
        )

        guard let uploadURL = created.uploadUrl else {
            throw APIError.invalidResponse
        }

        try await upload(
            payload,
            to: uploadURL,
            contentType: contentType,
            sha256: checksum
        )

        return try await authenticatedMutation(
            "/v1/transfers/\(created.id)/uploaded",
            idempotencyKey: "transfer-uploaded-\(created.id)"
        )
    }

    func receiveReadyTextItems(
        destinationDeviceID: String
    ) async throws -> [ReceivedTextItem] {
        let transfers = try await listTransfers()
        var received: [ReceivedTextItem] = []

        for transfer in transfers where
            transfer.destinationDeviceId == destinationDeviceID &&
            transfer.status == .ready &&
            [.text, .link, .clipboard].contains(transfer.kind) {
            let data = try await downloadPayload(for: transfer)
            guard let text = String(data: data, encoding: .utf8) else {
                throw APIError.invalidResponse
            }

            _ = try await completeTransfer(transfer.id)

            received.append(
                ReceivedTextItem(
                    id: transfer.id,
                    kind: transfer.kind,
                    text: text,
                    receivedAt: Date()
                )
            )
        }

        return received
    }

    func downloadPayload(for transfer: Transfer) async throws -> Data {
        guard let downloadURL = transfer.downloadUrl else {
            throw APIError.invalidResponse
        }

        let (data, response) = try await session.data(from: downloadURL)
        guard
            let http = response as? HTTPURLResponse,
            (200..<300).contains(http.statusCode)
        else {
            throw APIError.invalidResponse
        }

        guard sha256Hex(data).caseInsensitiveCompare(transfer.sha256) == .orderedSame else {
            throw APIError.checksumMismatch
        }
        guard Int64(data.count) == transfer.sizeBytes else {
            throw APIError.checksumMismatch
        }

        return data
    }

    func completeTransfer(_ transferID: String) async throws -> Transfer {
        try await authenticatedMutation(
            "/v1/transfers/\(transferID)/complete",
            idempotencyKey: "transfer-complete-\(transferID)"
        )
    }

    func openEvents(deviceID: String) async throws {
        guard let pair = try await sessionStore.load() else {
            throw APIError.noSession
        }

        closeEvents()

        guard var components = URLComponents(
            url: baseURL,
            resolvingAgainstBaseURL: false
        ) else {
            throw APIError.invalidResponse
        }
        components.scheme = baseURL.scheme == "https" ? "wss" : "ws"
        components.path = "/v1/events"
        components.queryItems = [
            URLQueryItem(name: "deviceId", value: deviceID)
        ]
        guard let url = components.url else {
            throw APIError.invalidResponse
        }

        var request = URLRequest(url: url)
        request.setValue(
            "\(pair.tokenType) \(pair.accessToken)",
            forHTTPHeaderField: "Authorization"
        )

        let task = session.webSocketTask(with: request)
        webSocketTask = task
        task.resume()
    }

    func nextEvent() async throws -> RealtimeEventEnvelope {
        guard let webSocketTask else {
            throw APIError.realtimeNotConnected
        }

        let message = try await webSocketTask.receive()
        let data: Data
        switch message {
        case .data(let value):
            data = value
        case .string(let value):
            guard let encoded = value.data(using: .utf8) else {
                throw APIError.invalidResponse
            }
            data = encoded
        @unknown default:
            throw APIError.invalidResponse
        }
        return try decoder.decode(RealtimeEventEnvelope.self, from: data)
    }

    func closeEvents() {
        webSocketTask?.cancel(with: .goingAway, reason: nil)
        webSocketTask = nil
    }

    func listDevices() async throws -> [PixelDevice] {
        try await authenticatedGet("/v1/devices")
    }

    func listTransfers() async throws -> [Transfer] {
        try await authenticatedGet("/v1/transfers")
    }

    func devicePresence(_ deviceID: String) async throws -> DevicePresence {
        try await authenticatedGet("/v1/presence/\(deviceID)")
    }

    private func authenticatedGet<T: Decodable>(_ path: String) async throws -> T {
        var request = URLRequest(url: baseURL.appending(path: path))
        request.httpMethod = "GET"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        return try await performAuthenticated(request, retryAfterRefresh: true)
    }

    private func authenticatedPost<Body: Encodable, Response: Decodable>(
        _ path: String,
        body: Body,
        idempotencyKey: String
    ) async throws -> Response {
        var request = URLRequest(url: baseURL.appending(path: path))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue(idempotencyKey, forHTTPHeaderField: "Idempotency-Key")
        request.httpBody = try encoder.encode(body)
        return try await performAuthenticated(request, retryAfterRefresh: true)
    }

    private func authenticatedMutation<Response: Decodable>(
        _ path: String,
        idempotencyKey: String
    ) async throws -> Response {
        var request = URLRequest(url: baseURL.appending(path: path))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue(idempotencyKey, forHTTPHeaderField: "Idempotency-Key")
        return try await performAuthenticated(request, retryAfterRefresh: true)
    }

    private func performAuthenticated<T: Decodable>(
        _ original: URLRequest,
        retryAfterRefresh: Bool
    ) async throws -> T {
        guard let pair = try await sessionStore.load() else {
            throw APIError.noSession
        }

        var request = original
        request.setValue(
            "\(pair.tokenType) \(pair.accessToken)",
            forHTTPHeaderField: "Authorization"
        )

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIError.invalidResponse
        }

        if http.statusCode == 401 && retryAfterRefresh {
            do {
                _ = try await refreshSession(
                    afterFailedAccessToken: pair.accessToken
                )
                return try await performAuthenticated(
                    original,
                    retryAfterRefresh: false
                )
            } catch {
                try? await sessionStore.clear()
                throw APIError.refreshFailed
            }
        }

        guard (200..<300).contains(http.statusCode) else {
            throw APIError.server(status: http.statusCode)
        }
        return try decoder.decode(T.self, from: data)
    }

    private func upload(
        _ data: Data,
        to url: URL,
        contentType: String,
        sha256: String
    ) async throws {
        var request = URLRequest(url: url)
        request.httpMethod = "PUT"
        request.setValue(contentType, forHTTPHeaderField: "Content-Type")
        request.setValue(String(data.count), forHTTPHeaderField: "Content-Length")
        request.setValue(sha256, forHTTPHeaderField: "X-Amz-Meta-Sha256")

        let (_, response) = try await session.upload(
            for: request,
            from: data
        )
        guard
            let http = response as? HTTPURLResponse,
            (200..<300).contains(http.statusCode)
        else {
            throw APIError.invalidResponse
        }
    }

    private func inferredTextKind(_ value: String) -> Transfer.Kind {
        guard
            let url = URL(string: value.trimmingCharacters(in: .whitespacesAndNewlines)),
            let scheme = url.scheme?.lowercased(),
            scheme == "http" || scheme == "https"
        else {
            return .text
        }
        return .link
    }

    private func sha256Hex(_ data: Data) -> String {
        SHA256.hash(data: data)
            .map { String(format: "%02x", $0) }
            .joined()
    }

    /// Coalesces simultaneous 401 responses into one refresh-token rotation.
    private func refreshSession(
        afterFailedAccessToken failedAccessToken: String
    ) async throws -> TokenPair {
        guard let current = try await sessionStore.load() else {
            throw APIError.noSession
        }

        if current.accessToken != failedAccessToken {
            return current
        }

        if let refreshTask {
            return try await refreshTask.value
        }

        let refreshToken = current.refreshToken
        let task = Task<TokenPair, Error> {
            try await self.performRefresh(refreshToken: refreshToken)
        }
        refreshTask = task

        do {
            let pair = try await task.value
            try await sessionStore.save(pair)
            refreshTask = nil
            return pair
        } catch {
            refreshTask = nil
            throw error
        }
    }

    private func performRefresh(refreshToken: String) async throws -> TokenPair {
        try await publicPost(
            "/v1/auth/refresh",
            body: RefreshRequest(refreshToken: refreshToken)
        )
    }

    private func publicPost<Body: Encodable, Response: Decodable>(
        _ path: String,
        body: Body
    ) async throws -> Response {
        var request = URLRequest(url: baseURL.appending(path: path))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try encoder.encode(body)

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            throw APIError.server(status: http.statusCode)
        }
        return try decoder.decode(Response.self, from: data)
    }
}
