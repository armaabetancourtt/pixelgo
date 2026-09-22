import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        Group {
            if !model.didBootstrap {
                ProgressView("Opening PIXEL GO…")
            } else if model.isAuthenticated {
                HomeView()
            } else {
                AuthView()
            }
        }
    }
}

private struct AuthView: View {
    @EnvironmentObject private var model: AppModel
    @State private var email = ""
    @State private var password = ""
    @State private var createAccount = false

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            Spacer()

            Text("PIXEL GO")
                .font(.system(size: 38, weight: .black, design: .rounded))
            Text("Your devices. One private transfer space.")
                .font(.title3)
                .foregroundStyle(.secondary)

            TextField("Email", text: $email)
                .textInputAutocapitalization(.never)
                .keyboardType(.emailAddress)
                .textContentType(.emailAddress)
                .padding()
                .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 16))

            SecureField("Password", text: $password)
                .textContentType(createAccount ? .newPassword : .password)
                .padding()
                .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 16))

            Button {
                Task {
                    if createAccount {
                        await model.register(email: email, password: password)
                    } else {
                        await model.login(email: email, password: password)
                    }
                }
            } label: {
                Group {
                    if model.isLoading {
                        ProgressView()
                    } else {
                        Text(createAccount ? "CREATE ACCOUNT" : "SIGN IN")
                    }
                }
                .font(.headline)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 14)
            }
            .buttonStyle(.borderedProminent)
            .disabled(email.isEmpty || password.isEmpty || model.isLoading)

            Button(
                createAccount
                    ? "Already have an account? Sign in"
                    : "New to PIXEL GO? Create account"
            ) {
                createAccount.toggle()
                model.errorMessage = nil
            }
            .frame(maxWidth: .infinity)

            if let error = model.errorMessage {
                Text(error)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .center)
            }

            Spacer()
        }
        .padding(28)
    }
}

private struct HomeView: View {
    @EnvironmentObject private var model: AppModel
    @State private var showingSend = false

    private var destinationDevices: [PixelDevice] {
        model.devices.filter { $0.id != model.localDeviceID }
    }

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
                                    .fill(
                                        model.onlineDeviceIDs.contains(device.id)
                                            ? Color.green
                                            : Color.gray
                                    )
                                    .frame(width: 8, height: 8)

                                VStack(alignment: .leading, spacing: 2) {
                                    HStack(spacing: 6) {
                                        Text(device.name)
                                            .font(.headline)
                                        if device.id == model.localDeviceID {
                                            Text("THIS DEVICE")
                                                .font(.system(size: 9, weight: .bold))
                                                .foregroundStyle(.secondary)
                                        }
                                    }

                                    Text(device.platform.uppercased())
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }

                                Spacer()

                                Text(
                                    model.onlineDeviceIDs.contains(device.id)
                                        ? "Online"
                                        : "Offline"
                                )
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            }
                        }
                    }
                }

                if !model.receivedItems.isEmpty {
                    Section("INBOX") {
                        ForEach(model.receivedItems) { item in
                            VStack(alignment: .leading, spacing: 5) {
                                Text(item.kind == .link ? "LINK" : "TEXT")
                                    .font(.caption2)
                                    .fontWeight(.bold)
                                    .foregroundStyle(.secondary)
                                Text(item.text)
                                    .font(.body)
                                    .textSelection(.enabled)
                                    .lineLimit(4)
                            }
                            .padding(.vertical, 2)
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
                                Text(
                                    transfer.displayName
                                        ?? transfer.kind.rawValue.capitalized
                                )
                                .font(.headline)

                                HStack {
                                    Text(
                                        ByteCountFormatter.string(
                                            fromByteCount: transfer.sizeBytes,
                                            countStyle: .file
                                        )
                                    )
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
                Button {
                    showingSend = true
                } label: {
                    Label("SEND", systemImage: "plus")
                        .font(.headline)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 14)
                }
                .buttonStyle(.borderedProminent)
                .disabled(destinationDevices.isEmpty || model.isLoading)
                .padding()
                .background(.ultraThinMaterial)
            }
            .sheet(isPresented: $showingSend) {
                SendTextView(destinations: destinationDevices)
                    .environmentObject(model)
            }
            .navigationBarHidden(true)
        }
    }

    private var brand: some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: 8) {
                Text("PIXEL GO")
                    .font(.system(size: 34, weight: .black, design: .rounded))
                Text("Native cross-device sharing.")
                    .foregroundStyle(.secondary)

                if destinationDevices.isEmpty {
                    Text("Sign in on another device to start sending.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                if let error = model.errorMessage {
                    Text(error)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }

            Spacer()

            Button("Sign out") {
                Task { await model.signOut() }
            }
            .font(.caption)
        }
        .padding(.vertical, 24)
    }
}

private struct SendTextView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss

    let destinations: [PixelDevice]

    @State private var selectedDeviceID: String
    @State private var text = ""

    init(destinations: [PixelDevice]) {
        self.destinations = destinations
        _selectedDeviceID = State(
            initialValue: destinations.first?.id ?? ""
        )
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("TO") {
                    Picker("Device", selection: $selectedDeviceID) {
                        ForEach(destinations) { device in
                            Text(device.name)
                                .tag(device.id)
                        }
                    }
                }

                Section("TEXT OR LINK") {
                    TextEditor(text: $text)
                        .frame(minHeight: 140)

                    Text(
                        "URLs are detected automatically and sent as link transfers."
                    )
                    .font(.caption)
                    .foregroundStyle(.secondary)
                }

                Section {
                    Button {
                        Task {
                            let sent = await model.sendText(
                                text,
                                to: selectedDeviceID
                            )
                            if sent {
                                dismiss()
                            }
                        }
                    } label: {
                        HStack {
                            Spacer()
                            if model.isLoading {
                                ProgressView()
                            } else {
                                Text("SEND")
                                    .fontWeight(.bold)
                            }
                            Spacer()
                        }
                    }
                    .disabled(
                        selectedDeviceID.isEmpty ||
                        text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ||
                        model.isLoading
                    )
                }
            }
            .navigationTitle("Send")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
    }
}
