import Foundation
import XCTest
@testable import PixelGoContracts

final class PixelGoContractsTests: XCTestCase {
    func testDecodesReadyPhotoTransfer() throws {
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
        XCTAssertEqual(transfer.displayName, "IMG_2048.jpg")
        XCTAssertEqual(transfer.sizeBytes, 42)
    }

    func testUploadAndDownloadURLsAreOptionalAcrossLifecycleStates() throws {
        let json = """
        {
          "id":"tr_2",
          "sourceDeviceId":"ios_1",
          "destinationDeviceId":"android_1",
          "kind":"file",
          "status":"completed",
          "sizeBytes":8,
          "sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
          "downloadUrl":"http://localhost:8080/dev-download/tr_2?exp=1&sig=abc",
          "createdAt":"2026-09-21T20:00:00Z",
          "updatedAt":"2026-09-21T20:00:02Z"
        }
        """.data(using: .utf8)!

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601

        let transfer = try decoder.decode(Transfer.self, from: json)

        XCTAssertNil(transfer.uploadUrl)
        XCTAssertNotNil(transfer.downloadUrl)
        XCTAssertEqual(transfer.status, .completed)
    }
}
