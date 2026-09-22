import Foundation

actor APIClient {
    enum APIError: LocalizedError {
        case invalidResponse
        case server(status: Int)
        case noSession
        case refreshFailed
        case realtimeNotConnected

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

        var request = URLRequest(url: baseURL.appending(path: "/v1/devices"))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue(idempotencyKey, forHTTPHeaderField: "Idempotency-Key")
        request.httpBody = try encoder.encode(body)

        let device: PixelDevice = try await performAuthenticated(
            request,
            retryAfterRefresh: true
        )
        try await sessionStore.saveDeviceID(device.id, for: pair.userId)
        return device
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

    /// Coalesces simultaneous 401 responses into one refresh-token rotation.
    ///
    /// Refresh tokens are one-time-use. Without single-flight coordination,
    /// two concurrent API calls could both attempt to rotate the same token;
    /// the second attempt would correctly look like token reuse and revoke
    /// the entire refresh family.
    private func refreshSession(
        afterFailedAccessToken failedAccessToken: String
    ) async throws -> TokenPair {
        guard let current = try await sessionStore.load() else {
            throw APIError.noSession
        }

        // Another request already refreshed while this request was in flight.
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
