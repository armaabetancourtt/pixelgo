import Foundation

actor APIClient {
    enum APIError: Error {
        case invalidResponse
        case server(status: Int)
    }

    private let baseURL: URL
    private let session: URLSession
    private let decoder: JSONDecoder

    init(baseURL: URL, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.session = session
        self.decoder = JSONDecoder()
        self.decoder.dateDecodingStrategy = .iso8601
    }

    func listDevices() async throws -> [PixelDevice] {
        try await get("/v1/devices")
    }

    func listTransfers() async throws -> [Transfer] {
        try await get("/v1/transfers")
    }

    func devicePresence(_ deviceID: String) async throws -> DevicePresence {
        try await get("/v1/presence/\(deviceID)")
    }

    private func get<T: Decodable>(_ path: String) async throws -> T {
        let url = baseURL.appending(path: path)
        var request = URLRequest(url: url)
        request.setValue("application/json", forHTTPHeaderField: "Accept")

        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            throw APIError.server(status: http.statusCode)
        }
        return try decoder.decode(T.self, from: data)
    }
}
