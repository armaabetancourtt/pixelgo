// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "PixelGoContracts",
    platforms: [
        .macOS(.v15)
    ],
    products: [
        .library(name: "PixelGoContracts", targets: ["PixelGoContracts"])
    ],
    targets: [
        .target(
            name: "PixelGoContracts",
            path: "PixelGo",
            exclude: [
                "APIClient.swift",
                "ContentView.swift",
                "Info.plist",
                "KeychainStore.swift",
                "PixelGoApp.swift",
                "TransferBackgroundCoordinator.swift"
            ],
            sources: ["Models.swift"]
        ),
        .testTarget(
            name: "PixelGoContractsTests",
            dependencies: ["PixelGoContracts"],
            path: "PixelGoContractTests"
        )
    ]
)
