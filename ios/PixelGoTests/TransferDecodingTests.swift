import XCTest
@testable import PixelGo

final class TransferDecodingTests: XCTestCase {
    func testDecodesTransfer() throws {
        let json = """
        {
          "id":"tr_1",
          "sourceDeviceId":"ios_1",
          "destinationDeviceId":"android_1",
          "kind":"photo",
          "status":"ready",
          "displayName":"IMG_2048.jpg",
          "sizeBytes":42,
          "sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          "createdAt":"2026-09-21T20:00:00Z",
          "updatedAt":"2026-09-21T20:00:01Z"
        }
        """.data(using: .utf8)!

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let transfer = try decoder.decode(Transfer.self, from: json)
        XCTAssertEqual(transfer.kind, .photo)
        XCTAssertEqual(transfer.status, .ready)
    }

    func testDecodesDevicePresence() throws {
        let json = """
        {
          "id":"dev_1",
          "name":"Armando's iPhone",
          "platform":"ios",
          "online":true,
          "createdAt":"2026-09-21T20:00:00Z"
        }
        """.data(using: .utf8)!

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let device = try decoder.decode(PixelDevice.self, from: json)

        XCTAssertTrue(device.online)
        XCTAssertEqual(device.platform, "ios")
    }
}
