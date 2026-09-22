import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        NavigationStack {
            List {
                brand
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)

                Section("YOUR DEVICES") {
                    if model.devices.isEmpty {
                        Text("No devices registered yet")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(model.devices) { device in
                            HStack(spacing: 12) {
                                Circle()
                                    .fill(model.onlineDeviceIDs.contains(device.id) ? Color.green : Color.gray)
                                    .frame(width: 8, height: 8)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(device.name)
                                        .font(.headline)
                                    Text(device.platform.uppercased())
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                                Spacer()
                                Text(model.onlineDeviceIDs.contains(device.id) ? "Online" : "Offline")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }

                Section("RECENT") {
                    if model.transfers.isEmpty {
                        Text("Nothing sent yet")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(model.transfers) { transfer in
                            VStack(alignment: .leading, spacing: 5) {
                                Text(transfer.displayName ?? transfer.kind.rawValue.capitalized)
                                    .font(.headline)
                                HStack {
                                    Text(ByteCountFormatter.string(fromByteCount: transfer.sizeBytes, countStyle: .file))
                                    Spacer()
                                    Text(transfer.status.rawValue)
                                }
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            .refreshable { await model.reload() }
            .safeAreaInset(edge: .bottom) {
                Button(action: {}) {
                    Label("SEND", systemImage: "plus")
                        .font(.headline)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 14)
                }
                .buttonStyle(.borderedProminent)
                .padding()
                .background(.ultraThinMaterial)
            }
            .navigationBarHidden(true)
        }
    }

    private var brand: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("PIXEL GO")
                .font(.system(size: 34, weight: .black, design: .rounded))
            Text("Native cross-device sharing.")
                .foregroundStyle(.secondary)
            if let error = model.errorMessage {
                Text(error)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 24)
    }
}
