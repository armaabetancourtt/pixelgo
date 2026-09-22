import Foundation

actor SessionStore {
    private let account = "pixelgo.session.v1"
    private let keychain: KeychainStore
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    init(keychain: KeychainStore = KeychainStore()) {
        self.keychain = keychain
    }

    func load() throws -> TokenPair? {
        guard let data = try keychain.read(account: account) else {
            return nil
        }
        return try decoder.decode(TokenPair.self, from: data)
    }

    func save(_ pair: TokenPair) throws {
        try keychain.save(encoder.encode(pair), account: account)
    }

    func clear() throws {
        try keychain.delete(account: account)
    }
}
