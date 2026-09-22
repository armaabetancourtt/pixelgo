import Foundation

actor SessionStore {
    private let sessionAccount = "pixelgo.session.v1"
    private let keychain: KeychainStore
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    init(keychain: KeychainStore = KeychainStore()) {
        self.keychain = keychain
    }

    func load() throws -> TokenPair? {
        guard let data = try keychain.read(account: sessionAccount) else {
            return nil
        }
        return try decoder.decode(TokenPair.self, from: data)
    }

    func save(_ pair: TokenPair) throws {
        try keychain.save(
            encoder.encode(pair),
            account: sessionAccount
        )
    }

    func clear() throws {
        try keychain.delete(account: sessionAccount)
    }

    func deviceID(for userID: String) throws -> String? {
        try readString(account: deviceAccount(userID))
    }

    func registrationKey(for userID: String) throws -> String {
        let account = registrationAccount(userID)
        if let existing = try readString(account: account) {
            return existing
        }

        let key = "device-register-\(UUID().uuidString.lowercased())"
        try saveString(key, account: account)
        return key
    }

    func saveDeviceID(_ deviceID: String, for userID: String) throws {
        try saveString(deviceID, account: deviceAccount(userID))
        try? keychain.delete(account: registrationAccount(userID))
    }

    private func deviceAccount(_ userID: String) -> String {
        "pixelgo.device.\(userID)"
    }

    private func registrationAccount(_ userID: String) -> String {
        "pixelgo.device.registration.\(userID)"
    }

    private func readString(account: String) throws -> String? {
        guard let data = try keychain.read(account: account) else {
            return nil
        }
        return String(data: data, encoding: .utf8)
    }

    private func saveString(_ value: String, account: String) throws {
        guard let data = value.data(using: .utf8) else {
            return
        }
        try keychain.save(data, account: account)
    }
}
