import Foundation

struct PixelDevice: Codable, Identifiable, Hashable {
    let id: String
    let name: String
    let platform: String
    let pushToken: String?
    let createdAt: Date
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
