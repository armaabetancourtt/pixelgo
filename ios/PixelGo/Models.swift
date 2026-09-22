import Foundation

struct TokenPair: Codable, Hashable {
    let userId: String
    let accessToken: String
    let refreshToken: String
    let tokenType: String
    let expiresInSeconds: Int64
}

struct AuthCredentials: Codable {
    let email: String
    let password: String
}

struct RefreshRequest: Codable {
    let refreshToken: String
}

struct PushTokenRequest: Codable {
    let pushToken: String
}

struct RegisterDeviceRequest: Codable {
    let name: String
    let platform: String
    let pushToken: String?
}

struct RealtimeEventEnvelope: Codable, Hashable {
    let type: String
}

struct PixelDevice: Codable, Identifiable, Hashable {
    let id: String
    let name: String
    let platform: String
    let pushToken: String?
    let createdAt: Date
}

struct DevicePresence: Codable, Hashable {
    let deviceId: String
    let online: Bool
}

struct CreateTransferRequest: Codable {
    let sourceDeviceId: String
    let destinationDeviceId: String
    let kind: String
    let displayName: String
    let contentType: String
    let sizeBytes: Int64
    let sha256: String
}

struct ReceivedTextItem: Identifiable, Hashable {
    let id: String
    let kind: Transfer.Kind
    let text: String
    let receivedAt: Date
}

struct Transfer: Codable, Identifiable, Hashable {
    enum Kind: String, Codable { case file, photo, link, text, clipboard }
    enum Status: String, Codable { case created, uploading, ready, downloading, completed, failed }

    let id: String
    let sourceDeviceId: String
    let destinationDeviceId: String
    let kind: Kind
    let status: Status
    let displayName: String?
    let contentType: String?
    let sizeBytes: Int64
    let sha256: String
    let uploadUrl: URL?
    let downloadUrl: URL?
    let createdAt: Date
    let updatedAt: Date
}
